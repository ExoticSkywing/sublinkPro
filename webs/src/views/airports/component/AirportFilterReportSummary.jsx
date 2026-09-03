import { useMemo, useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';

import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import FilterAltOutlinedIcon from '@mui/icons-material/FilterAltOutlined';
import { alpha, useTheme } from '@mui/material/styles';

import useResolvedColorScheme from 'hooks/useResolvedColorScheme';
import NodeFilterReportDialog from 'components/NodeFilterReportDialog';
import { formatDateTime } from '../utils';

const parseSummary = (value) => {
  if (!value) return null;
  if (typeof value === 'object') return value;
  if (typeof value !== 'string') return null;

  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === 'object' ? parsed : null;
  } catch {
    return null;
  }
};

const count = (value) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(0, parsed) : 0;
};

/**
 * 在机场列表中始终展示最近一次成功拉取的节点过滤摘要。
 */
export default function AirportFilterReportSummary({ airport, sx }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const { isDark } = useResolvedColorScheme();
  const [dialogOpen, setDialogOpen] = useState(false);
  const summary = useMemo(() => parseSummary(airport.nodeFilterSummary), [airport.nodeFilterSummary]);
  const warningTextColor = isDark ? theme.palette.warning.light : theme.palette.warning.contrastText;
  const warningBorderColor = isDark ? theme.palette.warning.light : theme.palette.warning.dark;

  if (!summary) return null;

  const total = count(summary.total);
  const filtered = count(summary.filtered);
  const retained = count(summary.retained);
  const globalFiltered = count(summary.globalFiltered);
  const airportFiltered = count(summary.airportFiltered);
  const reportTime = summary.generatedAt || airport.lastRunTime;

  return (
    <>
      <Box
        sx={[
          {
            p: 0.9,
            borderRadius: 1.5,
            bgcolor: alpha(theme.palette.warning.main, isDark ? 0.08 : 0.045),
            border: `1px solid ${alpha(theme.palette.warning.main, isDark ? 0.28 : 0.2)}`
          },
          sx
        ]}
      >
        <Stack
          direction="row"
          spacing={0.5}
          alignItems="center"
          justifyContent="space-between"
          useFlexGap
          flexWrap="wrap"
          sx={{ mb: 0.6, rowGap: 0.35 }}
        >
          <Stack direction="row" spacing={0.45} alignItems="center" sx={{ minWidth: 110, flex: '1 1 110px' }}>
            <FilterAltOutlinedIcon sx={{ fontSize: 14, color: warningTextColor }} />
            <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.primary', whiteSpace: 'nowrap' }}>
              {t('airports.filterReport.title')}
            </Typography>
          </Stack>
          {reportTime && (
            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.62rem', flex: '0 1 auto', whiteSpace: 'nowrap' }}>
              {t('airports.filterReport.lastRun', { time: formatDateTime(reportTime) })}
            </Typography>
          )}
        </Stack>

        <Stack direction="row" spacing={0.45} useFlexGap flexWrap="wrap" sx={{ mb: 0.7 }}>
          <Chip
            size="small"
            variant="outlined"
            label={t('airports.filterReport.total', { count: total })}
            sx={{ height: 20, fontSize: '0.64rem' }}
          />
          <Chip
            size="small"
            variant="outlined"
            color={filtered > 0 ? 'warning' : 'default'}
            label={t('airports.filterReport.filtered', { count: filtered })}
            sx={
              filtered > 0
                ? {
                    height: 20,
                    fontSize: '0.64rem',
                    color: warningTextColor,
                    borderColor: alpha(warningBorderColor, isDark ? 0.5 : 0.55),
                    '& .MuiChip-label': { color: warningTextColor }
                  }
                : { height: 20, fontSize: '0.64rem' }
            }
          />
          <Chip
            size="small"
            variant="outlined"
            color="success"
            label={t('airports.filterReport.retained', { count: retained })}
            sx={{ height: 20, fontSize: '0.64rem' }}
          />
          <Chip
            size="small"
            variant="outlined"
            label={t('airports.filterReport.breakdown', { global: globalFiltered, airport: airportFiltered })}
            sx={{ height: 20, fontSize: '0.64rem' }}
          />
        </Stack>

        <Button
          size="small"
          variant="text"
          color="warning"
          startIcon={<FilterAltOutlinedIcon sx={{ fontSize: '15px !important' }} />}
          onClick={(event) => {
            event.stopPropagation();
            setDialogOpen(true);
          }}
          sx={{
            minHeight: 26,
            px: 0.5,
            fontSize: '0.7rem',
            fontWeight: 600,
            color: warningTextColor,
            '& .MuiButton-startIcon': { color: warningTextColor },
            '&:hover': { bgcolor: alpha(warningBorderColor, isDark ? 0.14 : 0.08) }
          }}
        >
          {t('airports.filterReport.view', { count: filtered })}
        </Button>
      </Box>

      <NodeFilterReportDialog open={dialogOpen} onClose={() => setDialogOpen(false)} taskName={airport.name} summary={summary} />
    </>
  );
}

AirportFilterReportSummary.propTypes = {
  sx: PropTypes.oneOfType([PropTypes.object, PropTypes.array]),
  airport: PropTypes.shape({
    name: PropTypes.string,
    lastRunTime: PropTypes.oneOfType([PropTypes.string, PropTypes.instanceOf(Date)]),
    nodeFilterSummary: PropTypes.oneOfType([PropTypes.string, PropTypes.object])
  }).isRequired
};
