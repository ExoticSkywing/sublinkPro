package geoip

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sublink/config"
	"sublink/utils"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lionsoul2014/ip2region/binding/golang/service"
	"github.com/oschwald/geoip2-golang/v2"
	"golang.org/x/sync/singleflight"
)

var (
	geoIP          *geoip2.Reader
	mu             sync.RWMutex
	dbPath         string    // 当前加载的数据库路径
	available      bool      // 数据库是否可用
	dbInfo         *DBInfo   // 数据库信息
	initOnce       sync.Once // 确保只初始化一次
	cityAliases    *cityIndex
	cityGeneration atomic.Uint64
)

// DBInfo 数据库信息
type DBInfo struct {
	Path      string    `json:"path"`      // 文件路径
	Size      int64     `json:"size"`      // 文件大小（字节）
	ModTime   time.Time `json:"modTime"`   // 最后修改时间
	Available bool      `json:"available"` // 是否可用
}

// InitGeoIP 初始化 GeoIP 数据库
// 如果文件不存在，不会阻止系统启动，只是标记为不可用
func InitGeoIP() error {
	var initErr error
	initOnce.Do(func() {
		initErr = loadDatabase()
	})
	return initErr
}

// loadDatabase 加载数据库文件
func loadDatabase() error {
	mu.Lock()
	defer mu.Unlock()
	cityGeneration.Add(1)
	cityAliases = nil

	// 获取配置的路径
	path := config.GetGeoIPPath()
	dbPath = path

	// 检查文件是否存在
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		available = false
		dbInfo = &DBInfo{
			Path:      path,
			Available: false,
		}
		utils.Warn("GeoIP 数据库文件不存在: %s，相关功能将不可用", path)
		return nil // 不返回错误，允许系统继续启动
	}
	if err != nil {
		available = false
		dbInfo = &DBInfo{
			Path:      path,
			Available: false,
		}
		utils.Error("检查 GeoIP 数据库文件失败: %v", err)
		return nil
	}

	// 打开数据库
	reader, err := geoip2.Open(path)
	if err != nil {
		available = false
		dbInfo = &DBInfo{
			Path:      path,
			Size:      fileInfo.Size(),
			ModTime:   fileInfo.ModTime(),
			Available: false,
		}
		utils.Error("打开 GeoIP 数据库失败: %v", err)
		return err
	}

	// 关闭旧的 reader
	if geoIP != nil {
		if err := geoIP.Close(); err != nil {
			utils.Warn("关闭旧 GeoIP 数据库失败: %v", err)
		}
	}

	geoIP = reader
	if aliases, err := loadCityIndex(path); err == nil {
		cityAliases = aliases
	} else {
		utils.Warn("GeoIP 城市名称索引不可用，分发定位兜底将拒绝无法映射的城市")
	}
	available = true
	dbInfo = &DBInfo{
		Path:      path,
		Size:      fileInfo.Size(),
		ModTime:   fileInfo.ModTime(),
		Available: true,
	}

	utils.Info("GeoIP 数据库加载成功: %s (%.2f MB)", path, float64(fileInfo.Size())/1024/1024)
	return nil
}

// Reload 重新加载 GeoIP 数据库
func Reload() error {
	// 重置 initOnce 以允许重新初始化
	initOnce = sync.Once{}
	return loadDatabase()
}

// IsAvailable 检查 GeoIP 数据库是否可用
func IsAvailable() bool {
	mu.RLock()
	defer mu.RUnlock()
	return available
}

// GetDBInfo 获取数据库信息
func GetDBInfo() *DBInfo {
	mu.RLock()
	defer mu.RUnlock()

	if dbInfo != nil {
		return dbInfo
	}

	// 返回默认信息
	path := config.GetGeoIPPath()
	info := &DBInfo{
		Path:      path,
		Available: false,
	}

	// 尝试获取文件信息
	if fileInfo, err := os.Stat(path); err == nil {
		info.Size = fileInfo.Size()
		info.ModTime = fileInfo.ModTime()
	}

	return info
}

// GetDBPath 获取当前数据库路径
func GetDBPath() string {
	mu.RLock()
	defer mu.RUnlock()
	if dbPath != "" {
		return dbPath
	}
	return config.GetGeoIPPath()
}


var (
	localIp2RegionOnce sync.Once
	localIp2Region     *service.Ip2Region
)

func getOfflineIp2Region() *service.Ip2Region {
	localIp2RegionOnce.Do(func() {
		cfg := config.GetDistributionGeoIPSettings()
		var v4, v6 *service.Config
		var err error
		v4, err = service.NewV4Config(service.BufferCache, cfg.IPv4Path, 1)
		if err != nil {
			utils.Warn("IPv4 ip2region 未加载: %v", err)
		}
		v6, err = service.NewV6Config(service.BufferCache, cfg.IPv6Path, 1)
		if err != nil {
			utils.Warn("IPv6 ip2region 未加载: %v", err)
		}
		local, err := service.NewIp2Region(v4, v6)
		if err != nil {
			utils.Warn("ip2region 初始化失败: %v", err)
			return
		}
		localIp2Region = local
	})
	return localIp2Region
}

// formatCNRegion 规整中国省市格式（自动剔除直辖市/自治区后缀冗余）

type coffeeGeoResponse struct {
	Country     string `json:"country"`
	Region      string `json:"region"`
	City        string `json:"city"`
	ISP         string `json:"isp"`
	CountryCode string `json:"country_code"`
}

var (
	coffeeClient = &http.Client{Timeout: 1500 * time.Millisecond}
	coffeeFlight singleflight.Group
)

// fetchCoffeeGeo 当本地数据库无法定位城市或运营商时，作为在线备用源查询 ip.net.coffee
func fetchCoffeeGeo(ip string) *coffeeGeoResponse {
	val, err, _ := coffeeFlight.Do(ip, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", "https://ip.net.coffee/api/geoip/"+ip, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Referer", "https://ip.net.coffee/")

		resp, err := coffeeClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("status: %d", resp.StatusCode)
		}

		var data coffeeGeoResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return nil, err
		}
		return &data, nil
	})
	if err != nil || val == nil {
		return nil
	}
	return val.(*coffeeGeoResponse)
}

func formatCNRegion(prov, city string) string {
	cleanProv := prov
	for _, suffix := range []string{"特别行政区", "壮族自治区", "回族自治区", "维吾尔自治区", "自治区", "省", "市"} {
		cleanProv = strings.TrimSuffix(cleanProv, suffix)
	}
	cleanCity := strings.TrimSuffix(city, "市")

	if cleanProv != "" && cleanCity != "" {
		if cleanProv == cleanCity || strings.HasPrefix(cleanCity, cleanProv) {
			return cleanCity
		}
		return cleanProv + cleanCity
	}
	if cleanCity != "" {
		return cleanCity
	}
	return cleanProv
}

// GetLocation 返回给定 IP 地址的位置信息（融合 GeoLite2 与离线 ip2region 细粒度省市及运营商数据）
func GetLocation(ipStr string) (string, error) {
	mu.RLock()
	defer mu.RUnlock()

	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return "Unknown", nil
	}
	if ip.IsPrivate() || ip.IsLoopback() || !ip.IsGlobalUnicast() {
		return "内网/私有地址", nil
	}

	country := ""
	city := ""
	prov := ""
	isocode := ""

	if available && geoIP != nil {
		geoCountry, err := geoIP.Country(ip)
		if err != nil {
			utils.Error("Failed to get Country: %v", err)
		}
		if geoCountry.Country.HasData() {
			country = geoCountry.Country.Names.SimplifiedChinese
			isocode = geoCountry.Country.ISOCode
		}

		getCity, err := geoIP.City(ip)
		if err != nil {
			utils.Error("Failed to get City: %v", err)
		}
		if getCity.City.HasData() {
			city = getCity.City.Names.SimplifiedChinese
			if len(getCity.Subdivisions) > 0 {
				prov = getCity.Subdivisions[0].Names.SimplifiedChinese
			}
		}
	}

	// 离线 ip2region 兜底补充细粒度省市及运营商信息（零网络延迟）
	var isp string
	if local := getOfflineIp2Region(); local != nil {
		if region, err := local.Search(ipStr); err == nil {
			parts := strings.Split(region, "|")
			if len(parts) == 5 {
				rCountry := parts[0]
				rProv := parts[1]
				rCity := parts[2]
				rISP := parts[3]
				rISO := parts[4]

				if isocode == "" && rISO != "0" && len(rISO) == 2 {
					isocode = rISO
				}
				if country == "" && rCountry != "0" && rCountry != "Reserved" {
					country = rCountry
				}
				if prov == "" && rProv != "0" {
					prov = rProv
				}
				if city == "" && rCity != "0" {
					city = rCity
				}
				if isp == "" && rISP != "0" {
					isp = rISP
				}
			}
		}
	}

	// 若仍缺少城市或运营商信息，尝试从 ip.net.coffee 补充
	if (city == "" && prov == "") || isp == "" {
		if cGeo := fetchCoffeeGeo(ipStr); cGeo != nil {
			if isocode == "" && cGeo.CountryCode != "" {
				isocode = strings.ToUpper(cGeo.CountryCode)
			}
			if country == "" && cGeo.Country != "" {
				country = cGeo.Country
			}
			if prov == "" && cGeo.Region != "" {
				prov = cGeo.Region
			}
			if city == "" && cGeo.City != "" {
				city = cGeo.City
			}
			if isp == "" && cGeo.ISP != "" {
				isp = cGeo.ISP
			}
		}
	}

	if country == "" && city == "" && !available {
		return "", fmt.Errorf("GeoIP 数据库不可用")
	}

	flag := ISOCodeToFlag(isocode)
	var countryStr string
	if flag != "" {
		countryStr = fmt.Sprintf("%s%s", flag, country)
	} else if isocode != "" {
		countryStr = fmt.Sprintf("(%s)%s", isocode, country)
	} else {
		countryStr = country
	}

	var regionStr string
	if isocode == "CN" {
		regionStr = formatCNRegion(prov, city)
	} else {
		if city != "" {
			regionStr = city
		} else if prov != "" {
			regionStr = prov
		}
	}

	var ispStr string
	if isp != "" && isp != "0" {
		ispStr = fmt.Sprintf(" (%s)", isp)
	}

	res := fmt.Sprintf("%s%s%s", countryStr, regionStr, ispStr)
	if res == "" {
		res = "未知位置"
	}
	return res, nil
}

// ISOCodeToFlag 将 ISO 3166-1 alpha-2 国家代码转换为国旗 emoji
// 示例: "CN" -> 🇨🇳, "US" -> 🇺🇸
func ISOCodeToFlag(isoCode string) string {
	if len(isoCode) != 2 {
		return ""
	}

	// 将每个字母转换为对应的区域指示符号
	// 区域指示符号范围从 U+1F1E6 (A) 到 U+1F1FF (Z)
	flag := ""
	for _, char := range isoCode {
		if char >= 'A' && char <= 'Z' {
			flag += string(0x1F1E6 + (char - 'A'))
		} else if char >= 'a' && char <= 'z' {
			flag += string(0x1F1E6 + (char - 'a'))
		}
	}
	return flag
}

// GetCountryISOCode 返回给定 IP 地址的 ISO 国家代码 (例如 "US", "CN", "JP")
func GetCountryISOCode(ipStr string) (string, error) {
	mu.RLock()
	defer mu.RUnlock()

	if !available || geoIP == nil {
		return "", fmt.Errorf("GeoIP 数据库不可用")
	}

	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return "", fmt.Errorf("无效的 IP 地址: %s", ipStr)
	}

	geoCountry, err := geoIP.Country(ip)
	if err != nil {
		return "", fmt.Errorf("获取国家信息失败: %v", err)
	}
	if geoCountry.Country.HasData() {
		return geoCountry.Country.ISOCode, nil
	}
	return "", nil
}

// Close 关闭 GeoIP reader
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if geoIP != nil {
		err := geoIP.Close()
		geoIP = nil
		available = false
		return err
	}
	return nil
}
