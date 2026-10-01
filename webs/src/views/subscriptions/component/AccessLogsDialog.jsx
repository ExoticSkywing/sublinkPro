import Tooltip from '@mui/material/Tooltip';
import { useMemo, useState } from 'react';
import { useTheme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import useMediaQuery from '@mui/material/useMediaQuery';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Popover from '@mui/material/Popover';
import Button from '@mui/material/Button';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TableSortLabel from '@mui/material/TableSortLabel';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Stack from '@mui/material/Stack';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import TextField from '@mui/material/TextField';
import InputAdornment from '@mui/material/InputAdornment';
import IconButton from '@mui/material/IconButton';
import AccessTimeIcon from '@mui/icons-material/AccessTime';
import LocationOnIcon from '@mui/icons-material/LocationOn';
import TouchAppIcon from '@mui/icons-material/TouchApp';
import SearchIcon from '@mui/icons-material/Search';
import ClearIcon from '@mui/icons-material/Clear';
import DevicesIcon from '@mui/icons-material/Devices';
import PublicIcon from '@mui/icons-material/Public';
import MapIcon from '@mui/icons-material/Map';
import CellTowerIcon from '@mui/icons-material/CellTower';
import SignalCellularAltIcon from '@mui/icons-material/SignalCellularAlt';
import HubIcon from '@mui/icons-material/Hub';
import RouterIcon from '@mui/icons-material/Router';
import useResolvedColorScheme from 'hooks/useResolvedColorScheme';
import { getReadableTextTokens, getSurfaceTokens } from 'themes/surfaceTokens';
import { withAlpha } from 'utils/colorUtils';

const SORT_FIELDS = {
  ip: 'ip',
  client: 'client',
  region: 'region',
  count: 'count',
  date: 'date'
};

const SORT_ORDERS = {
  asc: 'asc',
  desc: 'desc'
};

const parseIPv4 = (value) => {
  const parts = String(value || '')
    .trim()
    .split('.');

  if (parts.length !== 4) return null;

  const octets = parts.map((part) => {
    if (!/^\d+$/.test(part)) return null;
    const octet = Number(part);
    return octet >= 0 && octet <= 255 ? octet : null;
  });

  return octets.every((octet) => octet !== null) ? octets : null;
};

const compareIPv4Octets = (left, right) => {
  for (let index = 0; index < left.length; index += 1) {
    if (left[index] !== right[index]) return left[index] - right[index];
  }

  return 0;
};

const compareIpValues = (leftValue, rightValue) => {
  const leftIp = String(leftValue || '').trim();
  const rightIp = String(rightValue || '').trim();
  const leftIPv4 = parseIPv4(leftIp);
  const rightIPv4 = parseIPv4(rightIp);

  if (leftIPv4 && rightIPv4) return compareIPv4Octets(leftIPv4, rightIPv4);
  if (leftIPv4) return -1;
  if (rightIPv4) return 1;

  return leftIp.localeCompare(rightIp, undefined, { numeric: true, sensitivity: 'base' });
};

const normalizeCount = (value) => Number(value) || 0;

const normalizeDate = (value) => {
  const timestamp = Date.parse(String(value || ''));
  return Number.isNaN(timestamp) ? 0 : timestamp;
};

const parseDomesticInfo = (addr) => {
  if (!addr) return null;
  const raw = String(addr).trim();
  if (!raw || raw.includes('内网') || raw.includes('私有地址')) return null;
  if (/香港|澳门|台湾|HK|MO|TW/i.test(raw)) return null;
  if (!/中国|🇨🇳/i.test(raw) && !/(?:^|[\s([/])CN/i.test(raw)) return null;

  let region = raw.replace(/^[\s🇨🇳CNcn中国]+/i, '').trim();
  region = region.replace(/[\(（][^\)）]+[\)）]\s*$/, '').trim();

  let isp = 'other';
  if (/电信|telecom/i.test(raw)) {
    isp = 'telecom';
  } else if (/移动|mobile/i.test(raw)) {
    isp = 'mobile';
  } else if (/联通|unicom/i.test(raw)) {
    isp = 'unicom';
  }

  return {
    region: region || '其他',
    isp,
    raw
  };
};

export default function AccessLogsDialog({ open, logs, onClose, loading = false, title }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  const [sortField, setSortField] = useState(SORT_FIELDS.date);
  const [sortOrder, setSortOrder] = useState(SORT_ORDERS.desc);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [regionAnchorEl, setRegionAnchorEl] = useState(null);
  const [regionFilterQuery, setRegionFilterQuery] = useState('');
  const isRegionPopoverOpen = Boolean(regionAnchorEl);
  const { isDark } = useResolvedColorScheme();
  const { palette, dialogSurface, dialogSurfaceGradient, mutedPanelSurface, nestedPanelSurface, panelBorder } = getSurfaceTokens(
    theme,
    isDark
  );
  const { primaryText, secondaryText, tertiaryText } = getReadableTextTokens(theme, isDark);

  const rowHoverSurface = withAlpha(palette.primary.main, isDark ? 0.12 : 0.05);
  const rowBorder = isDark ? withAlpha(palette.divider, 0.58) : withAlpha(palette.divider, 0.78);
  const ipSurface = isDark ? withAlpha(palette.primary.main, 0.14) : withAlpha(palette.primary.main, 0.06);
  const ipBorder = withAlpha(palette.primary.main, isDark ? 0.26 : 0.16);
  const countChipSurface = withAlpha(palette.primary.main, isDark ? 0.18 : 0.08);
  const countChipBorder = withAlpha(palette.primary.main, isDark ? 0.34 : 0.18);
  const sortControlSurface = isDark ? withAlpha(palette.background.paper, 0.72) : withAlpha(palette.background.default, 0.92);
  const sortFocusBorder = withAlpha(palette.primary.main, isDark ? 0.44 : 0.28);

  const unknownSourceLabel = t('subscriptions.accessLogs.unknownSource');

  const sortOptions = useMemo(
    () => [
      { value: SORT_FIELDS.ip, label: t('subscriptions.accessLogs.sort.fields.ip') },
      { value: SORT_FIELDS.client, label: t('subscriptions.accessLogs.sort.fields.client') },
      { value: SORT_FIELDS.region, label: t('subscriptions.accessLogs.sort.fields.region') },
      { value: SORT_FIELDS.count, label: t('subscriptions.accessLogs.sort.fields.count') },
      { value: SORT_FIELDS.date, label: t('subscriptions.accessLogs.sort.fields.date') }
    ],
    [t]
  );

  const filteredLogs = useMemo(() => {
    const keyword = searchKeyword.trim().toLowerCase();
    if (!keyword) return logs;

    return logs.filter((log) => {
      const ip = String(log.IP || '').toLowerCase();
      const client = String(log.Client || log.UA || '').toLowerCase();
      const region = String(log.Addr || unknownSourceLabel).toLowerCase();

      return ip.includes(keyword) || client.includes(keyword) || region.includes(keyword);
    });
  }, [logs, searchKeyword, unknownSourceLabel]);

  const sortedLogs = useMemo(() => {
    const compareLogs = (leftLog, rightLog) => {
      let comparison = 0;

      if (sortField === SORT_FIELDS.ip) {
        comparison = compareIpValues(leftLog.IP, rightLog.IP);
      } else if (sortField === SORT_FIELDS.client) {
        const leftClient = String(leftLog.Client || leftLog.UA || '');
        const rightClient = String(rightLog.Client || rightLog.UA || '');
        comparison = leftClient.localeCompare(rightClient, undefined, { numeric: true, sensitivity: 'base' });
      } else if (sortField === SORT_FIELDS.region) {
        const leftRegion = String(leftLog.Addr || unknownSourceLabel);
        const rightRegion = String(rightLog.Addr || unknownSourceLabel);
        comparison = leftRegion.localeCompare(rightRegion, undefined, { numeric: true, sensitivity: 'base' });
      } else if (sortField === SORT_FIELDS.count) {
        comparison = normalizeCount(leftLog.Count) - normalizeCount(rightLog.Count);
      } else if (sortField === SORT_FIELDS.date) {
        comparison = normalizeDate(leftLog.Date) - normalizeDate(rightLog.Date);
      }

      return sortOrder === SORT_ORDERS.desc ? comparison * -1 : comparison;
    };

    return filteredLogs
      .map((log, index) => ({ log, index }))
      .sort((left, right) => {
        const comparison = compareLogs(left.log, right.log);
        return comparison === 0 ? left.index - right.index : comparison;
      })
      .map(({ log }) => log);
  }, [filteredLogs, sortField, sortOrder, unknownSourceLabel]);

  const hasSearchKeyword = searchKeyword.trim().length > 0;

  const handleSort = (field) => {
    if (field === sortField) {
      setSortOrder((currentOrder) => (currentOrder === SORT_ORDERS.asc ? SORT_ORDERS.desc : SORT_ORDERS.asc));
      return;
    }

    setSortField(field);
    setSortOrder(field === SORT_FIELDS.count || field === SORT_FIELDS.date ? SORT_ORDERS.desc : SORT_ORDERS.asc);
  };

  const handleSortFieldChange = (field) => {
    setSortField(field);
    setSortOrder(field === SORT_FIELDS.count || field === SORT_FIELDS.date ? SORT_ORDERS.desc : SORT_ORDERS.asc);
  };

  const dialogPaperSx = {
    borderRadius: isMobile ? 0 : 3,
    overflow: 'hidden',
    bgcolor: dialogSurface,
    backgroundImage: dialogSurfaceGradient,
    border: '1px solid',
    borderColor: panelBorder
  };

  const titleSx = {
    px: 2.5,
    py: 2,
    bgcolor: mutedPanelSurface,
    borderBottom: '1px solid',
    borderColor: panelBorder,
    boxShadow: `inset 0 -1px 0 ${withAlpha(palette.divider, 0.42)}`
  };

  const actionsSx = {
    px: 2.5,
    py: 1.5,
    bgcolor: mutedPanelSurface,
    borderTop: '1px solid',
    borderColor: panelBorder
  };

  const countChipSx = {
    ml: 1,
    bgcolor: countChipSurface,
    color: palette.primary.main,
    border: '1px solid',
    borderColor: countChipBorder,
    fontWeight: 600
  };

  const mobileSortControlSx = {
    minWidth: 0,
    flex: 1,
    '& .MuiInputLabel-root': {
      color: secondaryText
    },
    '& .MuiOutlinedInput-root': {
      bgcolor: sortControlSurface,
      color: primaryText,
      borderRadius: 2,
      '& fieldset': { borderColor: rowBorder },
      '&:hover fieldset': { borderColor: sortFocusBorder },
      '&.Mui-focused fieldset': { borderColor: sortFocusBorder }
    },
    '& .MuiSelect-icon': {
      color: secondaryText
    }
  };

  const directionToggleSx = {
    flexShrink: 0,
    bgcolor: sortControlSurface,
    borderRadius: 2,
    '& .MuiToggleButton-root': {
      minWidth: 48,
      px: 1.25,
      color: secondaryText,
      borderColor: rowBorder,
      '&.Mui-selected': {
        color: palette.primary.main,
        bgcolor: withAlpha(palette.primary.main, isDark ? 0.18 : 0.1)
      },
      '&.Mui-selected:hover': {
        bgcolor: withAlpha(palette.primary.main, isDark ? 0.24 : 0.14)
      }
    }
  };

  const searchFieldSx = {
    mb: 1.5,
    '& .MuiOutlinedInput-root': {
      bgcolor: sortControlSurface,
      color: primaryText,
      borderRadius: 2,
      '& fieldset': { borderColor: rowBorder },
      '&:hover fieldset': { borderColor: sortFocusBorder },
      '&.Mui-focused fieldset': { borderColor: sortFocusBorder }
    },
    '& .MuiInputBase-input::placeholder': {
      color: tertiaryText,
      opacity: 1
    }
  };

  const renderSortableHeader = (field, label, sx, align) => (
    <TableCell sx={{ whiteSpace: 'nowrap', ...sx }} align={align} sortDirection={sortField === field ? sortOrder : false}>
      <TableSortLabel
        active={sortField === field}
        direction={sortField === field ? sortOrder : SORT_ORDERS.asc}
        onClick={() => handleSort(field)}
        sx={{ whiteSpace: 'nowrap', '& .MuiTableSortLabel-icon': { flexShrink: 0 } }}
      >
        {label}
      </TableSortLabel>
    </TableCell>
  );

  const telecomColor = '#1890ff';
  const mobileColor = '#10b981';
  const unicomColor = '#fa8c16';
  const otherIspColor = '#8c8c8c';

  const telecomTextColor = isDark ? '#69c0ff' : '#096dd9';
  const mobileTextColor = isDark ? '#4ade80' : '#059669';
  const unicomTextColor = isDark ? '#ffc069' : '#d46b08';
  const otherIspTextColor = isDark ? '#bfbfbf' : '#595959';

  const domesticStats = useMemo(() => {
    let domesticTotal = 0;
    const regionMap = {};
    let telecomCount = 0;
    let mobileCount = 0;
    let unicomCount = 0;
    let otherCount = 0;

    (logs || []).forEach((log) => {
      const info = parseDomesticInfo(log.Addr);
      if (!info) return;

      domesticTotal += 1;
      regionMap[info.region] = (regionMap[info.region] || 0) + 1;

      if (info.isp === 'telecom') {
        telecomCount += 1;
      } else if (info.isp === 'mobile') {
        mobileCount += 1;
      } else if (info.isp === 'unicom') {
        unicomCount += 1;
      } else {
        otherCount += 1;
      }
    });

    const regionEntries = Object.entries(regionMap).sort((a, b) => b[1] - a[1]);
    const regionItems = regionEntries.map(([name, count]) => ({ name, count }));

    return {
      domesticTotal,
      regionCount: regionEntries.length,
      regionItems,
      telecomCount,
      mobileCount,
      unicomCount,
      otherCount
    };
  }, [logs]);

  const filteredRegionItems = useMemo(() => {
    const q = regionFilterQuery.trim().toLowerCase();
    if (!q) return domesticStats.regionItems;
    return domesticStats.regionItems.filter((item) => item.name.toLowerCase().includes(q));
  }, [domesticStats.regionItems, regionFilterQuery]);

  const renderStatCard = ({ key, icon, label, value, tooltip, color, textColor, active, onClick }) => {
    const cardContent = (
      <Box
        onClick={onClick}
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          px: 1.25,
          py: 0.85,
          borderRadius: 2,
          bgcolor: active ? withAlpha(color, isDark ? 0.28 : 0.16) : withAlpha(color, isDark ? 0.12 : 0.05),
          border: '1px solid',
          borderColor: active ? color : withAlpha(color, isDark ? 0.32 : 0.18),
          boxShadow: active ? `0 0 0 1px ${color}` : 'none',
          cursor: onClick ? 'pointer' : 'default',
          userSelect: 'none',
          transition: 'all 0.18s ease',
          '&:hover': onClick
            ? {
                bgcolor: withAlpha(color, isDark ? 0.22 : 0.12),
                borderColor: color
              }
            : {}
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0 }}>
          <Box sx={{ color, display: 'flex', alignItems: 'center', flexShrink: 0 }}>{icon}</Box>
          <Typography
            variant="body2"
            sx={{
              fontSize: '0.8rem',
              color: secondaryText,
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis'
            }}
          >
            {label}
          </Typography>
        </Box>
        <Typography
          variant="subtitle2"
          sx={{
            fontWeight: 700,
            fontSize: '0.85rem',
            color: textColor || color,
            ml: 0.75,
            whiteSpace: 'nowrap',
            flexShrink: 0
          }}
        >
          {value}
        </Typography>
      </Box>
    );

    return tooltip ? (
      <Tooltip key={key} title={tooltip} placement="top" arrow>
        <Box sx={key === 'other' ? { gridColumn: { xs: 'span 2', sm: 'auto' } } : undefined}>{cardContent}</Box>
      </Tooltip>
    ) : (
      <Box key={key} sx={key === 'other' ? { gridColumn: { xs: 'span 2', sm: 'auto' } } : undefined}>
        {cardContent}
      </Box>
    );
  };

  const renderStatsSummary = () => {
    if (!logs || logs.length === 0) return null;

    const isDomesticFiltered = searchKeyword.trim() === '中国';
    const isTelecomFiltered = searchKeyword.trim() === '电信';
    const isMobileFiltered = searchKeyword.trim() === '移动';
    const isUnicomFiltered = searchKeyword.trim() === '联通';

    const statItems = [
      {
        key: 'region',
        icon: <MapIcon sx={{ fontSize: 17 }} />,
        label: t('subscriptions.accessLogs.stats.domesticRegions'),
        value: t('subscriptions.accessLogs.stats.regionCount', { count: domesticStats.regionCount }),
        tooltip: t('subscriptions.accessLogs.stats.regionsTooltip', {
          count: domesticStats.regionCount
        }),
        color: palette.primary.main,
        textColor: palette.primary.main,
        active: isDomesticFiltered || Boolean(regionAnchorEl),
        onClick: (event) => setRegionAnchorEl(event.currentTarget)
      },
      {
        key: 'telecom',
        icon: <CellTowerIcon sx={{ fontSize: 17 }} />,
        label: t('subscriptions.accessLogs.stats.telecom'),
        value: t('subscriptions.accessLogs.stats.itemCount', { count: domesticStats.telecomCount }),
        tooltip: t('subscriptions.accessLogs.stats.filterTooltip', {
          name: t('subscriptions.accessLogs.stats.telecom'),
          count: domesticStats.telecomCount
        }),
        color: telecomColor,
        textColor: telecomTextColor,
        active: isTelecomFiltered,
        onClick: () => setSearchKeyword((prev) => (prev.trim() === '电信' ? '' : '电信'))
      },
      {
        key: 'mobile',
        icon: <SignalCellularAltIcon sx={{ fontSize: 17 }} />,
        label: t('subscriptions.accessLogs.stats.mobile'),
        value: t('subscriptions.accessLogs.stats.itemCount', { count: domesticStats.mobileCount }),
        tooltip: t('subscriptions.accessLogs.stats.filterTooltip', {
          name: t('subscriptions.accessLogs.stats.mobile'),
          count: domesticStats.mobileCount
        }),
        color: mobileColor,
        textColor: mobileTextColor,
        active: isMobileFiltered,
        onClick: () => setSearchKeyword((prev) => (prev.trim() === '移动' ? '' : '移动'))
      },
      {
        key: 'unicom',
        icon: <HubIcon sx={{ fontSize: 17 }} />,
        label: t('subscriptions.accessLogs.stats.unicom'),
        value: t('subscriptions.accessLogs.stats.itemCount', { count: domesticStats.unicomCount }),
        tooltip: t('subscriptions.accessLogs.stats.filterTooltip', {
          name: t('subscriptions.accessLogs.stats.unicom'),
          count: domesticStats.unicomCount
        }),
        color: unicomColor,
        textColor: unicomTextColor,
        active: isUnicomFiltered,
        onClick: () => setSearchKeyword((prev) => (prev.trim() === '联通' ? '' : '联通'))
      }
    ];

    if (domesticStats.otherCount > 0) {
      statItems.push({
        key: 'other',
        icon: <RouterIcon sx={{ fontSize: 17 }} />,
        label: t('subscriptions.accessLogs.stats.otherIsp'),
        value: t('subscriptions.accessLogs.stats.itemCount', { count: domesticStats.otherCount }),
        color: otherIspColor,
        textColor: otherIspTextColor,
        active: false,
        onClick: undefined
      });
    }

    return (
      <Box
        sx={{
          mb: 1.5,
          p: { xs: 1.25, sm: 1.5 },
          borderRadius: 2.5,
          bgcolor: nestedPanelSurface,
          border: '1px solid',
          borderColor: rowBorder
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1.25 }}>
          <Stack direction="row" alignItems="center" spacing={0.75}>
            <PublicIcon sx={{ fontSize: 16, color: palette.primary.main }} />
            <Typography variant="subtitle2" sx={{ fontWeight: 600, color: primaryText, fontSize: '0.85rem' }}>
              {t('subscriptions.accessLogs.stats.title')}
            </Typography>
            <Chip
              size="small"
              label={t('subscriptions.accessLogs.stats.domesticTotal', { count: domesticStats.domesticTotal })}
              sx={{
                height: 20,
                fontSize: '0.72rem',
                fontWeight: 600,
                bgcolor: withAlpha(palette.primary.main, isDark ? 0.18 : 0.08),
                color: palette.primary.main,
                border: '1px solid',
                borderColor: withAlpha(palette.primary.main, isDark ? 0.32 : 0.16)
              }}
            />
          </Stack>
          {hasSearchKeyword && (
            <Typography
              variant="caption"
              onClick={() => setSearchKeyword('')}
              sx={{
                color: palette.primary.main,
                fontWeight: 500,
                cursor: 'pointer',
                userSelect: 'none',
                '&:hover': { textDecoration: 'underline' }
              }}
            >
              {t('subscriptions.accessLogs.stats.clearFilter')}
            </Typography>
          )}
        </Box>
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: {
              xs: 'repeat(2, 1fr)',
              sm: `repeat(${statItems.length}, 1fr)`
            },
            gap: 1
          }}
        >
          {statItems.map(renderStatCard)}
        </Box>

        <Popover
          open={isRegionPopoverOpen}
          anchorEl={regionAnchorEl}
          onClose={() => {
            setRegionAnchorEl(null);
            setRegionFilterQuery('');
          }}
          anchorOrigin={{
            vertical: 'bottom',
            horizontal: 'left'
          }}
          transformOrigin={{
            vertical: 'top',
            horizontal: 'left'
          }}
          slotProps={{
            paper: {
              sx: {
                mt: 1,
                p: 2,
                width: { xs: 'calc(100vw - 32px)', sm: 460 },
                maxWidth: 520,
                borderRadius: 2.5,
                bgcolor: dialogSurface,
                backgroundImage: dialogSurfaceGradient,
                border: '1px solid',
                borderColor: panelBorder,
                boxShadow: isDark ? `0 14px 34px ${withAlpha(theme.palette.common.black, 0.45)}` : '0 10px 28px rgba(0,0,0,0.12)'
              }
            }
          }}
        >
          <Box sx={{ mb: 1.5, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Stack direction="row" alignItems="center" spacing={0.75}>
              <MapIcon sx={{ fontSize: 18, color: palette.primary.main }} />
              <Typography variant="subtitle2" sx={{ fontWeight: 600, color: primaryText, fontSize: '0.875rem' }}>
                {t('subscriptions.accessLogs.stats.regionsPopoverTitle')}
              </Typography>
              <Chip
                size="small"
                label={`${domesticStats.regionCount} 地区 · ${domesticStats.domesticTotal} IP`}
                sx={{
                  height: 20,
                  fontSize: '0.72rem',
                  fontWeight: 600,
                  bgcolor: withAlpha(palette.primary.main, isDark ? 0.18 : 0.08),
                  color: palette.primary.main,
                  border: '1px solid',
                  borderColor: withAlpha(palette.primary.main, isDark ? 0.32 : 0.16)
                }}
              />
            </Stack>
            <IconButton
              size="small"
              onClick={() => {
                setRegionAnchorEl(null);
                setRegionFilterQuery('');
              }}
            >
              <ClearIcon fontSize="small" />
            </IconButton>
          </Box>

          <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1.5 }}>
            <TextField
              fullWidth
              size="small"
              value={regionFilterQuery}
              onChange={(e) => setRegionFilterQuery(e.target.value)}
              placeholder={t('subscriptions.accessLogs.stats.searchRegionPlaceholder')}
              slotProps={{
                input: {
                  startAdornment: (
                    <InputAdornment position="start">
                      <SearchIcon fontSize="small" sx={{ color: tertiaryText }} />
                    </InputAdornment>
                  ),
                  endAdornment: regionFilterQuery ? (
                    <InputAdornment position="end">
                      <IconButton size="small" edge="end" onClick={() => setRegionFilterQuery('')}>
                        <ClearIcon fontSize="small" />
                      </IconButton>
                    </InputAdornment>
                  ) : null,
                  sx: {
                    height: 32,
                    fontSize: '0.8rem',
                    bgcolor: sortControlSurface,
                    borderRadius: 1.5,
                    '& fieldset': { borderColor: rowBorder }
                  }
                }
              }}
            />
            <Chip
              size="small"
              label={`${t('subscriptions.accessLogs.stats.allDomestic')} (${domesticStats.domesticTotal})`}
              onClick={() => {
                setSearchKeyword('中国');
                setRegionAnchorEl(null);
                setRegionFilterQuery('');
              }}
              variant={searchKeyword.trim() === '中国' ? 'filled' : 'outlined'}
              color="primary"
              sx={{ height: 30, fontWeight: 600, cursor: 'pointer', flexShrink: 0 }}
            />
          </Stack>

          <Box
            sx={{
              maxHeight: 260,
              overflowY: 'auto',
              display: 'flex',
              flexWrap: 'wrap',
              gap: 0.85,
              p: 0.25,
              pr: 0.5,
              '&::-webkit-scrollbar': { width: 5 },
              '&::-webkit-scrollbar-thumb': {
                bgcolor: withAlpha(palette.divider, 0.6),
                borderRadius: 2
              }
            }}
          >
            {filteredRegionItems.length === 0 ? (
              <Typography variant="body2" sx={{ color: secondaryText, py: 2, width: '100%', textAlign: 'center' }}>
                未找到匹配的地区
              </Typography>
            ) : (
              filteredRegionItems.map(({ name, count }) => {
                const isSelected = searchKeyword.trim() === name;
                return (
                  <Chip
                    key={name}
                    size="small"
                    label={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.6 }}>
                        <span style={{ fontWeight: isSelected ? 600 : 500 }}>{name}</span>
                        <Box
                          component="span"
                          sx={{
                            px: 0.6,
                            py: 0.1,
                            borderRadius: 1,
                            fontSize: '0.72rem',
                            fontWeight: 700,
                            bgcolor: isSelected
                              ? withAlpha(theme.palette.common.white, 0.28)
                              : withAlpha(palette.primary.main, isDark ? 0.25 : 0.12),
                            color: isSelected ? 'inherit' : palette.primary.main
                          }}
                        >
                          {count}
                        </Box>
                      </Box>
                    }
                    onClick={() => {
                      setSearchKeyword(isSelected ? '' : name);
                      setRegionAnchorEl(null);
                      setRegionFilterQuery('');
                    }}
                    variant={isSelected ? 'filled' : 'outlined'}
                    color={isSelected ? 'primary' : 'default'}
                    sx={{
                      height: 28,
                      cursor: 'pointer',
                      borderRadius: 1.5,
                      borderColor: isSelected ? palette.primary.main : rowBorder,
                      '&:hover': {
                        borderColor: palette.primary.main,
                        bgcolor: withAlpha(palette.primary.main, isDark ? 0.18 : 0.08)
                      }
                    }}
                  />
                );
              })
            )}
          </Box>
        </Popover>
      </Box>
    );
  };

  const renderSearchField = () => (
    <TextField
      fullWidth
      size="small"
      value={searchKeyword}
      onChange={(event) => setSearchKeyword(event.target.value)}
      placeholder={t('subscriptions.accessLogs.search.placeholder')}
      sx={searchFieldSx}
      slotProps={{
        htmlInput: {
          'aria-label': t('subscriptions.accessLogs.search.label')
        },
        input: {
          startAdornment: (
            <InputAdornment position="start">
              <SearchIcon fontSize="small" sx={{ color: tertiaryText }} />
            </InputAdornment>
          ),
          endAdornment: searchKeyword ? (
            <InputAdornment position="end">
              <IconButton
                size="small"
                edge="end"
                onClick={() => setSearchKeyword('')}
                aria-label={t('subscriptions.accessLogs.search.clear')}
              >
                <ClearIcon fontSize="small" />
              </IconButton>
            </InputAdornment>
          ) : null
        }
      }}
    />
  );

  const FilteredEmptyState = () => (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        py: 6,
        borderRadius: 2.5,
        bgcolor: nestedPanelSurface,
        border: '1px solid',
        borderColor: rowBorder,
        color: secondaryText
      }}
    >
      <SearchIcon sx={{ fontSize: 42, mb: 1.5, opacity: 0.5, color: tertiaryText }} />
      <Typography sx={{ color: secondaryText }}>{t('subscriptions.accessLogs.search.empty')}</Typography>
    </Box>
  );

  const MobileSortControls = () => (
    <Stack
      direction="row"
      spacing={1}
      sx={{
        mb: 1.5,
        minHeight: 40,
        alignItems: 'center'
      }}
    >
      <FormControl size="small" sx={mobileSortControlSx}>
        <InputLabel id="access-logs-sort-field-label">{t('subscriptions.accessLogs.sort.field')}</InputLabel>
        <Select
          labelId="access-logs-sort-field-label"
          value={sortField}
          label={t('subscriptions.accessLogs.sort.field')}
          onChange={(event) => handleSortFieldChange(event.target.value)}
        >
          {sortOptions.map((option) => (
            <MenuItem key={option.value} value={option.value}>
              {option.label}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <ToggleButtonGroup
        exclusive
        size="small"
        value={sortOrder}
        onChange={(_, nextOrder) => {
          if (nextOrder) setSortOrder(nextOrder);
        }}
        aria-label={t('subscriptions.accessLogs.sort.direction')}
        sx={directionToggleSx}
      >
        <ToggleButton value={SORT_ORDERS.asc} aria-label={t('subscriptions.accessLogs.sort.ascending')}>
          {t('subscriptions.accessLogs.sort.ascShort')}
        </ToggleButton>
        <ToggleButton value={SORT_ORDERS.desc} aria-label={t('subscriptions.accessLogs.sort.descending')}>
          {t('subscriptions.accessLogs.sort.descShort')}
        </ToggleButton>
      </ToggleButtonGroup>
    </Stack>
  );

  const renderClientBlock = (client, ua) => {
    const displayClient =
      client ||
      (ua ? (ua.length > 20 ? `${ua.slice(0, 20)}...` : ua) : t('subscriptions.accessLogs.unknownClient', { defaultValue: '未知' }));
    return (
      <Tooltip title={ua || displayClient} placement="top" arrow>
        <Chip
          size="small"
          icon={<DevicesIcon sx={{ fontSize: '13px !important' }} />}
          label={displayClient}
          sx={{
            height: 24,
            fontSize: '0.75rem',
            fontWeight: 500,
            bgcolor: withAlpha(palette.primary.main, isDark ? 0.16 : 0.08),
            color: palette.primary.main,
            border: '1px solid',
            borderColor: withAlpha(palette.primary.main, isDark ? 0.3 : 0.18),
            maxWidth: 160,
            '& .MuiChip-label': { px: 0.75, overflow: 'hidden', textOverflow: 'ellipsis' }
          }}
        />
      </Tooltip>
    );
  };

  const renderIpBlock = (ip) => (
    <Box
      sx={{
        display: 'inline-flex',
        alignItems: 'center',
        maxWidth: '100%',
        px: 1,
        py: 0.5,
        borderRadius: 1.25,
        bgcolor: ipSurface,
        border: '1px solid',
        borderColor: ipBorder
      }}
    >
      <Typography
        variant="body2"
        sx={{
          fontFamily: 'monospace',
          color: palette.primary.main,
          fontWeight: 600,
          wordBreak: 'break-all'
        }}
      >
        {ip}
      </Typography>
    </Box>
  );

  const MobileLogCard = ({ log }) => (
    <Card
      sx={{
        mb: 1.5,
        borderRadius: 2.5,
        bgcolor: nestedPanelSurface,
        border: '1px solid',
        borderColor: rowBorder,
        transition: 'all 0.2s ease',
        '&:hover': {
          bgcolor: rowHoverSurface,
          borderColor: withAlpha(palette.primary.main, isDark ? 0.24 : 0.14)
        }
      }}
    >
      <CardContent sx={{ py: 1.5, px: 2, '&:last-child': { pb: 1.5 } }}>
        <Stack spacing={1}>
          <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 1 }}>
            <Box sx={{ flex: 1, minWidth: 0 }}>{renderIpBlock(log.IP)}</Box>
            <Chip
              size="small"
              label={t('subscriptions.accessLogs.count', { count: log.Count })}
              icon={<TouchAppIcon sx={{ fontSize: 14 }} />}
              sx={{
                height: 24,
                bgcolor: countChipSurface,
                color: palette.primary.main,
                border: '1px solid',
                borderColor: countChipBorder,
                '& .MuiChip-label': { px: 1 },
                flexShrink: 0
              }}
            />
          </Box>

          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <DevicesIcon sx={{ fontSize: 16, color: tertiaryText, flexShrink: 0 }} />
            {renderClientBlock(log.Client, log.UA)}
          </Box>

          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <LocationOnIcon sx={{ fontSize: 16, color: tertiaryText, flexShrink: 0 }} />
            <Typography
              variant="body2"
              sx={{
                color: secondaryText,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap'
              }}
            >
              {log.Addr || unknownSourceLabel}
            </Typography>
          </Box>

          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <AccessTimeIcon sx={{ fontSize: 16, color: tertiaryText, flexShrink: 0 }} />
            <Typography variant="body2" sx={{ color: secondaryText }}>
              {log.Date}
            </Typography>
          </Box>
        </Stack>
      </CardContent>
    </Card>
  );

  const DesktopTable = () => (
    <TableContainer
      sx={{
        borderRadius: 2.5,
        bgcolor: nestedPanelSurface,
        border: '1px solid',
        borderColor: rowBorder,
        overflow: 'hidden'
      }}
    >
      <Table size="small">
        <TableHead>
          <TableRow
            sx={{
              bgcolor: mutedPanelSurface,
              '& .MuiTableCell-root': {
                borderColor: rowBorder
              }
            }}
          >
            {renderSortableHeader(SORT_FIELDS.ip, t('subscriptions.accessLogs.ip'), {
              fontWeight: 600,
              minWidth: 140,
              color: secondaryText
            })}
            {renderSortableHeader(SORT_FIELDS.client, t('subscriptions.accessLogs.client'), {
              fontWeight: 600,
              minWidth: 130,
              color: secondaryText
            })}
            {renderSortableHeader(SORT_FIELDS.region, t('subscriptions.accessLogs.region'), {
              fontWeight: 600,
              minWidth: 120,
              color: secondaryText
            })}
            {renderSortableHeader(
              SORT_FIELDS.count,
              t('subscriptions.accessLogs.visits'),
              { fontWeight: 600, width: 120, minWidth: 120, color: secondaryText },
              'center'
            )}
            {renderSortableHeader(SORT_FIELDS.date, t('subscriptions.accessLogs.lastVisit'), {
              fontWeight: 600,
              minWidth: 160,
              color: secondaryText
            })}
          </TableRow>
        </TableHead>
        <TableBody>
          {sortedLogs.map((log) => (
            <TableRow
              key={log.ID}
              sx={{
                transition: 'background-color 0.2s ease',
                '&:hover': { bgcolor: rowHoverSurface },
                '& .MuiTableCell-root': {
                  borderColor: rowBorder
                }
              }}
            >
              <TableCell>{renderIpBlock(log.IP)}</TableCell>
              <TableCell>{renderClientBlock(log.Client, log.UA)}</TableCell>
              <TableCell>
                <Typography variant="body2" sx={{ color: secondaryText }}>
                  {log.Addr || unknownSourceLabel}
                </Typography>
              </TableCell>
              <TableCell align="center" sx={{ width: 120, minWidth: 120, whiteSpace: 'nowrap' }}>
                <Chip size="small" label={log.Count} sx={{ ...countChipSx, minWidth: 50 }} />
              </TableCell>
              <TableCell>
                <Typography variant="body2" sx={{ color: secondaryText }}>
                  {log.Date}
                </Typography>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      fullScreen={isMobile}
      slotProps={{
        paper: {
          sx: dialogPaperSx
        }
      }}
    >
      <DialogTitle sx={titleSx}>
        <Stack direction="row" alignItems="center" spacing={1}>
          <TouchAppIcon sx={{ color: palette.primary.main }} />
          <Typography variant="h6" sx={{ color: primaryText }}>
            {title || t('subscriptions.accessLogs.title')}
          </Typography>
          {!loading && logs.length > 0 && (
            <Chip
              size="small"
              label={
                hasSearchKeyword
                  ? t('subscriptions.accessLogs.filteredTotal', { filtered: sortedLogs.length, total: logs.length })
                  : t('subscriptions.accessLogs.total', { count: logs.length })
              }
              sx={countChipSx}
            />
          )}
        </Stack>
      </DialogTitle>
      <DialogContent
        sx={{
          px: isMobile ? 1.5 : 2,
          pt: isMobile ? 2 : 2.5,
          pb: isMobile ? 1.5 : 2,
          bgcolor: dialogSurface,
          '&&': {
            pt: isMobile ? 2 : 2.5
          }
        }}
      >
        {loading ? (
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', py: 8 }}>
            <CircularProgress size={28} />
          </Box>
        ) : logs.length === 0 ? (
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              py: 8,
              borderRadius: 2.5,
              bgcolor: nestedPanelSurface,
              border: '1px solid',
              borderColor: rowBorder,
              color: secondaryText
            }}
          >
            <TouchAppIcon sx={{ fontSize: 48, mb: 2, opacity: 0.5, color: tertiaryText }} />
            <Typography sx={{ color: secondaryText }}>{t('subscriptions.accessLogs.empty')}</Typography>
          </Box>
        ) : isMobile ? (
          <Box>
            {renderStatsSummary()}
            {renderSearchField()}
            <MobileSortControls />
            {sortedLogs.length > 0 ? sortedLogs.map((log) => <MobileLogCard key={log.ID} log={log} />) : <FilteredEmptyState />}
          </Box>
        ) : (
          <Box>
            {renderStatsSummary()}
            {renderSearchField()}
            {sortedLogs.length > 0 ? <DesktopTable /> : <FilteredEmptyState />}
          </Box>
        )}
      </DialogContent>
      <DialogActions sx={actionsSx}>
        <Button onClick={onClose} variant="outlined">
          {t('common.close')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
