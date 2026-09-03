import { useEffect, useMemo, useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';

import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Select from '@mui/material/Select';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TablePagination from '@mui/material/TablePagination';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useTheme } from '@mui/material/styles';

import useResolvedColorScheme from 'hooks/useResolvedColorScheme';
import { getTaskCenterTokens, getTaskDialogPaperSx } from 'components/taskCenterTheme';

const PAGE_SIZE_OPTIONS = [25, 50, 100];

const getCount = (value) => (Number.isFinite(Number(value)) ? Number(value) : 0);

export default function NodeFilterReportDialog({ open, onClose, taskName, summary }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const { isDark } = useResolvedColorScheme();
  const tokens = getTaskCenterTokens(theme, isDark);
  const [stage, setStage] = useState('all');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(PAGE_SIZE_OPTIONS[0]);

  useEffect(() => {
    if (!open) return;
    setStage('all');
    setSearch('');
    setPage(0);
  }, [open, summary]);

  const filteredNodes = useMemo(() => {
    const nodes = Array.isArray(summary?.nodes) ? summary.nodes : [];
    const keyword = search.trim().toLowerCase();
    return nodes.filter((node) => {
      if (stage !== 'all' && node.stage !== stage) return false;
      if (!keyword) return true;
      return [
        node.name,
        node.protocol,
        node.reason,
        t(`tasks.filterReport.reason.${node.reason}`, { defaultValue: node.reason || '' }),
        t(`tasks.filterReport.stage.${node.stage}`, { defaultValue: node.stage || '' })
      ].some((value) => `${value || ''}`.toLowerCase().includes(keyword));
    });
  }, [summary?.nodes, search, stage, t]);

  useEffect(() => {
    const maxPage = Math.max(0, Math.ceil(filteredNodes.length / pageSize) - 1);
    setPage((currentPage) => Math.min(currentPage, maxPage));
  }, [filteredNodes.length, pageSize]);

  const visibleNodes = filteredNodes.slice(page * pageSize, (page + 1) * pageSize);

  const stageLabel = (value) => {
    if (value === 'global') return t('tasks.filterReport.stage.global');
    if (value === 'airport') return t('tasks.filterReport.stage.airport');
    return value || '-';
  };

  const reasonLabel = (value) => t(`tasks.filterReport.reason.${value}`, { defaultValue: value || '-' });

  const total = getCount(summary?.total);
  const filtered = getCount(summary?.filtered);
  const retained = getCount(summary?.retained);
  const globalFiltered = getCount(summary?.globalFiltered);
  const airportFiltered = getCount(summary?.airportFiltered);

  return (
    <Dialog
      open={open}
      onClose={onClose}
      aria-labelledby="node-filter-report-dialog-title"
      fullWidth
      maxWidth="md"
      PaperProps={{ sx: getTaskDialogPaperSx(theme, tokens, theme.palette.warning.main) }}
    >
      <DialogTitle id="node-filter-report-dialog-title" sx={{ color: tokens.primaryText }}>
        {t('tasks.filterReport.title')}
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2}>
          {taskName && (
            <Typography variant="body2" sx={{ color: tokens.secondaryText }}>
              {t('tasks.filterReport.taskName', { name: taskName })}
            </Typography>
          )}

          <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap">
            <Chip label={t('tasks.filterReport.total', { count: total })} color="primary" variant="outlined" />
            <Chip
              label={t('tasks.filterReport.filtered', { count: filtered })}
              color={filtered > 0 ? 'warning' : 'default'}
              variant="outlined"
            />
            <Chip label={t('tasks.filterReport.retained', { count: retained })} color="success" variant="outlined" />
            <Chip label={t('tasks.filterReport.globalFiltered', { count: globalFiltered })} variant="outlined" />
            <Chip label={t('tasks.filterReport.airportFiltered', { count: airportFiltered })} variant="outlined" />
          </Stack>

          {filtered === 0 ? (
            <Alert severity="info">{t('tasks.filterReport.none')}</Alert>
          ) : (
            <>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
                <TextField
                  size="small"
                  fullWidth
                  value={search}
                  onChange={(event) => {
                    setSearch(event.target.value);
                    setPage(0);
                  }}
                  label={t('tasks.filterReport.search')}
                  placeholder={t('tasks.filterReport.searchPlaceholder')}
                />
                <FormControl size="small" sx={{ minWidth: { xs: '100%', sm: 160 } }}>
                  <InputLabel id="node-filter-report-stage-label">{t('tasks.filterReport.stageLabel')}</InputLabel>
                  <Select
                    labelId="node-filter-report-stage-label"
                    value={stage}
                    label={t('tasks.filterReport.stageLabel')}
                    onChange={(event) => {
                      setStage(event.target.value);
                      setPage(0);
                    }}
                  >
                    <MenuItem value="all">{t('tasks.filterReport.allStages')}</MenuItem>
                    <MenuItem value="global">{t('tasks.filterReport.stage.global')}</MenuItem>
                    <MenuItem value="airport">{t('tasks.filterReport.stage.airport')}</MenuItem>
                  </Select>
                </FormControl>
              </Stack>

              {visibleNodes.length > 0 ? (
                <TableContainer sx={{ maxHeight: 420 }}>
                  <Table size="small" stickyHeader>
                    <TableHead>
                      <TableRow>
                        <TableCell>{t('tasks.filterReport.columns.name')}</TableCell>
                        <TableCell>{t('tasks.filterReport.columns.stage')}</TableCell>
                        <TableCell>{t('tasks.filterReport.columns.reason')}</TableCell>
                        <TableCell>{t('tasks.filterReport.columns.protocol')}</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {visibleNodes.map((node, index) => (
                        <TableRow key={`${node.name || 'node'}-${node.stage || 'stage'}-${index}`} hover>
                          <TableCell sx={{ maxWidth: 280, wordBreak: 'break-word' }}>{node.name || '-'}</TableCell>
                          <TableCell>{stageLabel(node.stage)}</TableCell>
                          <TableCell>{reasonLabel(node.reason)}</TableCell>
                          <TableCell>{node.protocol || '-'}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              ) : (
                <Alert severity="info">{t('tasks.filterReport.noMatches')}</Alert>
              )}

              <TablePagination
                component="div"
                count={filteredNodes.length}
                page={page}
                onPageChange={(_, nextPage) => setPage(nextPage)}
                rowsPerPage={pageSize}
                onRowsPerPageChange={(event) => {
                  setPageSize(Number(event.target.value));
                  setPage(0);
                }}
                rowsPerPageOptions={PAGE_SIZE_OPTIONS}
                labelRowsPerPage={t('tasks.filterReport.rowsPerPage')}
              />
            </>
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('common.close')}</Button>
      </DialogActions>
    </Dialog>
  );
}

NodeFilterReportDialog.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  taskName: PropTypes.string,
  summary: PropTypes.shape({
    total: PropTypes.number,
    retained: PropTypes.number,
    filtered: PropTypes.number,
    globalFiltered: PropTypes.number,
    airportFiltered: PropTypes.number,
    generatedAt: PropTypes.string,
    nodes: PropTypes.arrayOf(
      PropTypes.shape({
        name: PropTypes.string,
        protocol: PropTypes.string,
        stage: PropTypes.string,
        reason: PropTypes.string
      })
    )
  })
};
