import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useSearchParams } from 'react-router-dom';
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  MenuItem,
  Paper,
  Stack,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  Tabs,
  TextField,
  Typography
} from '@mui/material';
import MainCard from 'ui-component/cards/MainCard';
import { IconPlayerPause, IconPlayerPlay, IconPencil, IconTrash } from '@tabler/icons-react';
import { withAlpha } from 'utils/colorUtils';
import { distributionAPI } from 'api/distribution';
import request from 'api/request';
import {
  credentialState,
  cardDeleteButtonSx,
  dateText,
  errorText,
  ExportDialog,
  StatusChip,
  subscriptionURL,
  distributionSurfaceSx
} from './components/Common';
import CredentialDialog from './components/CredentialDialog';
import CredentialDeleteDialog from './components/CredentialDeleteDialog';
import CardDialog from './components/CardDialog';
import CardConfirmDialog from './components/CardConfirmDialog';
import CopyValueButton from './components/CopyValueButton';
import InlineCredentialName from './components/InlineCredentialName';
import RecordReference from './components/RecordReference';
import IssueDialog from './components/IssueDialog';
import ReviewDialog from './components/ReviewDialog';
import SettingsPanel from './components/SettingsPanel';
import { useRegionRequests } from './RegionRequestContext';

const resources = ['credentials', 'cards', 'region-requests', 'redemptions', 'settings'];
const tabKeys = ['credentials', 'cards', 'regionApplications', 'redemptions', 'settings'];

export default function DistributionManagement() {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  const tab = Math.max(0, resources.indexOf(searchParams.get('tab')));
  const { count: pendingCount, error: pendingError, refresh: refreshPending } = useRegionRequests();
  const [page, setPage] = useState(0);
  const [keyword, setKeyword] = useState('');
  const [filter, setFilter] = useState(tab === 2 ? 'pending' : '');
  const [data, setData] = useState({ items: [], total: 0 });
  const [settings, setSettings] = useState(null);
  const [subscriptions, setSubscriptions] = useState([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [selected, setSelected] = useState([]);
  const [issue, setIssue] = useState(false);
  const [exportValue, setExportValue] = useState('');
  const [credential, setCredential] = useState(null);
  const [credentialDeletion, setCredentialDeletion] = useState(null);
  const [card, setCard] = useState(null);
  const [cardConfirmation, setCardConfirmation] = useState(null);
  const [application, setApplication] = useState(null);
  const [revision, setRevision] = useState(0);
  const refresh = () => {
    setRevision((value) => value + 1);
    refreshPending();
  };
  useEffect(() => {
    setPage(0);
    setKeyword('');
    setFilter(tab === 2 ? 'pending' : '');
    setSelected([]);
    setData({ items: [], total: 0 });
  }, [tab, location.key]);
  // New applications and reviews in another tab also refresh the open list.
  const pendingRevision = tab === 2 ? pendingCount : null;
  useEffect(() => {
    Promise.all([distributionAPI.settings(), request.get('/v1/subcription/get')])
      .then(([cfg, subs]) => {
        setSettings(cfg.data);
        setSubscriptions(Array.isArray(subs.data) ? subs.data : subs.data?.items || []);
      })
      .catch((err) => setError(errorText(err, t)));
  }, [t]);
  useEffect(() => {
    if (tab === 4) return;
    let active = true;
    setBusy(true);
    const timer = setTimeout(async () => {
      setError('');
      try {
        const result = await distributionAPI.list(resources[tab], { page: page + 1, size: 20, keyword, status: filter });
        if (active) {
          if (page > 0 && result.data.items.length === 0) setPage(page - 1);
          setData(result.data);
          setSelected([]);
        }
      } catch (err) {
        if (active) setError(errorText(err, t));
      } finally {
        if (active) setBusy(false);
      }
    }, 200);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [tab, page, keyword, filter, t, revision, pendingRevision]);
  const perform = async (action, reload = true) => {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await action();
      if (reload) refresh();
    } catch (err) {
      setError(errorText(err, t));
    } finally {
      setBusy(false);
    }
  };
  const exportRows = (rows) =>
    setExportValue({
      text: rows.map((r) => `${r.name}\t${tab === 0 ? subscriptionURL(r.token, settings) : r.code}`).join('\n'),
      codes: tab === 1 ? rows.map((r) => r.code).join('\n') : undefined
    });
  const toggle = (id) => setSelected((items) => (items.includes(id) ? items.filter((v) => v !== id) : [...items, id]));
  const selectable = tab < 2 && !(tab === 0 && filter === 'revoked');
  const columns =
    tab === 0
      ? ['name', 'status', 'resourceSubscription', 'expiresAt', 'allowedCities', 'lastAccess', 'actions']
      : tab === 1
        ? ['name', 'plan', 'status', 'cardDeadline', 'redemptionCount', 'binding', 'actions']
        : tab === 2
          ? ['credential', 'targetCity', 'reason', 'status', 'createdAt', 'actions']
          : ['credential', 'card', 'plan', 'beforeExpiry', 'afterExpiry', 'source', 'createdAt'];
  return (
    <MainCard title={t('distribution.title')} sx={distributionSurfaceSx}>
      <Stack spacing={3}>
        <Typography color="text.secondary">{t('distribution.description')}</Typography>
        <Tabs
          value={tab}
          onChange={(_, value) => {
            setSearchParams({ tab: resources[value] });
          }}
          variant="scrollable"
          scrollButtons="auto"
          aria-label={t('distribution.title')}
        >
          {tabKeys.map((key, i) => (
            <Tab
              key={key}
              id={`distribution-tab-${i}`}
              aria-controls="distribution-panel"
              label={
                <Stack component="span" direction="row" spacing={1} alignItems="center">
                  <span>{t(`distribution.${key}`)}</span>
                  {i === 2 && pendingCount > 0 && (
                    <Chip
                      component="span"
                      size="small"
                      color="error"
                      label={t('distribution.pendingRegionsShort', { count: pendingCount })}
                    />
                  )}
                </Stack>
              }
            />
          ))}
        </Tabs>
        {error && <Alert severity="error">{error}</Alert>}
        {pendingError && (
          <Alert severity="warning" action={<Button onClick={refreshPending}>{t('distribution.refresh')}</Button>}>
            {t('distribution.pendingRegionsFailed')}
          </Alert>
        )}
        <Box role="status" aria-live="polite">
          {notice && (
            <Alert severity="success" role="presentation">
              {notice}
            </Alert>
          )}
        </Box>
        <Box role="tabpanel" id="distribution-panel" aria-labelledby={`distribution-tab-${tab}`}>
          {tab === 4 ? (
            settings && <SettingsPanel initial={settings} subscriptions={subscriptions} onSaved={setSettings} />
          ) : (
            <Stack spacing={2}>
              <Stack direction={{ xs: 'column', sm: 'row' }} gap={2} alignItems={{ sm: 'center' }} flexWrap="wrap">
                {tab < 2 && (
                  <TextField
                    size="small"
                    label={t(tab === 0 ? 'distribution.searchCredentials' : 'distribution.searchCards')}
                    autoComplete="off"
                    value={keyword}
                    onChange={(e) => {
                      setKeyword(e.target.value);
                      setPage(0);
                    }}
                    sx={{ minWidth: 192, flexGrow: 1 }}
                  />
                )}
                {tab < 3 && (
                  <TextField
                    select
                    size="small"
                    label={t('distribution.status')}
                    value={filter}
                    onChange={(e) => {
                      setFilter(e.target.value);
                      setPage(0);
                    }}
                    sx={{ minWidth: 128 }}
                  >
                    <MenuItem value="">{t(tab === 0 ? 'distribution.notDeleted' : 'distribution.all')}</MenuItem>
                    {(tab === 0
                      ? ['enabled', 'disabled', 'revoked']
                      : tab === 1
                        ? ['enabled', 'disabled']
                        : ['pending', 'approved', 'rejected']
                    ).map((s) => (
                      <MenuItem key={s} value={s}>
                        {t(`distribution.states.${s}`)}
                      </MenuItem>
                    ))}
                  </TextField>
                )}
                <Button variant="outlined" onClick={refresh} disabled={busy}>
                  {t('distribution.refresh')}
                </Button>
                {tab < 2 && (
                  <Button variant="contained" onClick={() => setIssue(true)} disabled={!settings || busy}>
                    {t(tab === 0 ? 'distribution.issueCredentials' : 'distribution.issueCards')}
                  </Button>
                )}
                {tab === 1 && (
                  <Button
                    variant="outlined"
                    disabled={busy}
                    onClick={() =>
                      perform(async () => {
                        try {
                          const result = await distributionAPI.currentFreeCard();
                          exportRows([result.data]);
                        } catch (err) {
                          if (err?.response?.data?.msg === 'card_deleted') setCardConfirmation({ cards: [], reissue: true });
                          else throw err;
                        }
                      })
                    }
                  >
                    {t('distribution.currentFreeCard')}
                  </Button>
                )}
              </Stack>
              {selected.length > 0 && selectable && (
                <Paper variant="outlined" sx={{ p: 2, bgcolor: 'background.default' }}>
                  <Stack direction="row" gap={2} alignItems="center" flexWrap="wrap">
                    <Typography>{t('distribution.selected', { count: selected.length })}</Typography>
                    <Button variant="outlined" onClick={() => exportRows(data.items.filter((r) => selected.includes(r.id)))}>
                      {t('distribution.exportSelected')}
                    </Button>
                    <Button
                      disabled={busy}
                      onClick={() =>
                        perform(async () => {
                          for (const id of selected) {
                            if (tab === 0) {
                              if (data.items.find((r) => r.id === id)?.status !== 'revoked')
                                await distributionAPI.updateCredential(id, { status: 'disabled' });
                            } else await distributionAPI.updateCard(id, { enabled: false });
                          }
                          setNotice(t('distribution.saved'));
                        })
                      }
                    >
                      {t('distribution.disableSelected')}
                    </Button>
                    {tab < 2 && (
                      <Button
                        color="error"
                        sx={cardDeleteButtonSx}
                        startIcon={<IconTrash size={18} />}
                        disabled={busy}
                        onClick={() => {
                          const rows = data.items.filter((r) => selected.includes(r.id));
                          if (tab === 0) setCredentialDeletion(rows);
                          else setCardConfirmation({ cards: rows });
                        }}
                      >
                        {t('distribution.deleteSelected')}
                      </Button>
                    )}
                  </Stack>
                </Paper>
              )}
              <TableContainer component={Paper} variant="outlined">
                <Table size="small" sx={{ minWidth: 768 }} aria-label={t(`distribution.${tabKeys[tab]}`)}>
                  <TableHead>
                    <TableRow>
                      {selectable && (
                        <TableCell padding="checkbox">
                          <Checkbox
                            checked={data.items.length > 0 && selected.length === data.items.length}
                            indeterminate={selected.length > 0 && selected.length < data.items.length}
                            onChange={(e) => setSelected(e.target.checked ? data.items.map((r) => r.id) : [])}
                            slotProps={{ input: { 'aria-label': t('distribution.selectPage') } }}
                          />
                        </TableCell>
                      )}
                      {columns.map((column) => (
                        <TableCell key={column} sx={{ py: 2, bgcolor: 'background.default', whiteSpace: 'nowrap' }}>
                          {t(`distribution.${column}`)}
                        </TableCell>
                      ))}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {data.items.map((r) => (
                      <TableRow key={r.id} hover>
                        {selectable && (
                          <TableCell padding="checkbox">
                            <Checkbox
                              checked={selected.includes(r.id)}
                              onChange={() => toggle(r.id)}
                              slotProps={{ input: { 'aria-label': `${t('distribution.select')} ${r.name}` } }}
                            />
                          </TableCell>
                        )}
                        {tab === 0 && (
                          <>
                            <TableCell>
                              <InlineCredentialName
                                credential={r}
                                onSaved={(updated) => {
                                  setData((current) => ({
                                    ...current,
                                    items: current.items.map((item) => (item.id === updated.id ? updated : item))
                                  }));
                                  setNotice(t('distribution.nameSaved'));
                                }}
                              />
                              <Typography variant="caption" color="text.secondary" display="block">
                                #{r.id}
                              </Typography>
                            </TableCell>
                            <TableCell>
                              <StatusChip value={credentialState(r)} />
                            </TableCell>
                            <TableCell>{subscriptions.find((s) => s.ID === r.subscription_id)?.Name || `#${r.subscription_id}`}</TableCell>
                            <TableCell>{r.permanent ? t('distribution.states.permanent') : dateText(r.expires_at)}</TableCell>
                            <TableCell>
                              {(r.regions || []).map((region) => region.city).join(' / ') || '—'}
                              <Typography variant="caption" display="block" color="text.secondary">
                                {r.regions?.length || 0}/{settings?.region_limit ?? 2}
                              </Typography>
                            </TableCell>
                            <TableCell>
                              {dateText(r.last_access_at)}
                              <Typography variant="caption" display="block" color="text.secondary">
                                {t('distribution.accessCount', { count: r.access_count })}
                              </Typography>
                            </TableCell>
                            <TableCell>
                              <Stack direction="row" spacing={1} sx={{ width: 'max-content', alignItems: 'center' }}>
                                {r.status !== 'revoked' && (
                                  <CopyValueButton
                                    key={subscriptionURL(r.token, settings)}
                                    value={settings ? subscriptionURL(r.token, settings) : ''}
                                    showFormats
                                  />
                                )}
                                <Button size="small" onClick={() => setCredential(r)} sx={{ minHeight: { xs: 44, sm: 36 } }}>
                                  {t('distribution.manage')}
                                </Button>
                                {r.status !== 'revoked' && (
                                  <Button
                                    size="small"
                                    color="error"
                                    startIcon={<IconTrash size={18} />}
                                    disabled={busy}
                                    onClick={() => setCredentialDeletion([r])}
                                    sx={[cardDeleteButtonSx, { minHeight: { xs: 44, sm: 40 } }]}
                                  >
                                    {t('distribution.delete')}
                                  </Button>
                                )}
                              </Stack>
                            </TableCell>
                          </>
                        )}
                        {tab === 1 && (
                          <>
                            <TableCell>
                              {r.name}
                              <Typography variant="caption" display="block" color="text.secondary">
                                #{r.id} · {r.batch}
                              </Typography>
                            </TableCell>
                            <TableCell>{t(`distribution.plans.${r.kind}`)}</TableCell>
                            <TableCell>
                              <StatusChip value={r.enabled ? 'enabled' : 'disabled'} />
                            </TableCell>
                            <TableCell>{dateText(r.ends_at)}</TableCell>
                            <TableCell>{r.redemption_count}</TableCell>
                            <TableCell>{r.bound_credential_id ? `#${r.bound_credential_id}` : '—'}</TableCell>
                            <TableCell>
                              <Stack direction="row" gap={1} sx={{ width: 320, flexWrap: 'wrap', alignItems: 'center', py: 1 }}>
                                <CopyValueButton key={r.code} value={r.code || ''} kind="card" />
                                <Button
                                  size="small"
                                  variant="outlined"
                                  startIcon={<IconPencil size={18} />}
                                  onClick={() => setCard(r)}
                                  disabled={busy}
                                  sx={{ minHeight: { xs: 44, sm: 40 } }}
                                >
                                  {t('distribution.edit')}
                                </Button>
                                <Button
                                  size="small"
                                  variant="outlined"
                                  color="inherit"
                                  startIcon={r.enabled ? <IconPlayerPause size={18} /> : <IconPlayerPlay size={18} />}
                                  disabled={busy}
                                  onClick={() =>
                                    perform(async () => {
                                      await distributionAPI.updateCard(r.id, { enabled: !r.enabled });
                                      setNotice(t(r.enabled ? 'distribution.cardDisabled' : 'distribution.cardEnabled'));
                                    })
                                  }
                                  sx={(theme) => {
                                    const palette = theme.vars?.palette || theme.palette;
                                    const accent = palette[r.enabled ? 'warning' : 'success'].main;
                                    return {
                                      minHeight: { xs: 44, sm: 40 },
                                      color: 'text.primary',
                                      borderColor: 'text.secondary',
                                      bgcolor: withAlpha(accent, 0.16),
                                      '&:hover': { bgcolor: withAlpha(accent, 0.24), borderColor: 'text.primary' }
                                    };
                                  }}
                                >
                                  {t(r.enabled ? 'distribution.disable' : 'distribution.enable')}
                                </Button>
                                <Button size="small" onClick={() => exportRows([r])} sx={{ minHeight: { xs: 44, sm: 40 } }}>
                                  {t('distribution.export')}
                                </Button>
                                <Button
                                  size="small"
                                  color="error"
                                  startIcon={<IconTrash size={18} />}
                                  disabled={busy}
                                  onClick={() => setCardConfirmation({ cards: [r] })}
                                  sx={[cardDeleteButtonSx, { minHeight: { xs: 44, sm: 40 } }]}
                                >
                                  {t('distribution.delete')}
                                </Button>
                              </Stack>
                            </TableCell>
                          </>
                        )}
                        {tab === 2 && (
                          <>
                            <TableCell>
                              <Button
                                onClick={() =>
                                  perform(async () => {
                                    const result = await distributionAPI.getCredential(r.credential_id);
                                    setCredential(result.data);
                                  })
                                }
                              >
                                #{r.credential_id}
                              </Button>
                            </TableCell>
                            <TableCell>
                              {r.province} · {r.city}
                            </TableCell>
                            <TableCell sx={{ maxWidth: 256, overflowWrap: 'anywhere' }}>{r.reason}</TableCell>
                            <TableCell>
                              <StatusChip value={r.status} />
                            </TableCell>
                            <TableCell>{dateText(r.created_at)}</TableCell>
                            <TableCell>
                              {r.status === 'pending' ? (
                                <Button size="small" variant="contained" onClick={() => setApplication(r)}>
                                  {t('distribution.review')}
                                </Button>
                              ) : (
                                r.reply || '—'
                              )}
                            </TableCell>
                          </>
                        )}
                        {tab === 3 && (
                          <>
                            <TableCell>
                              <RecordReference
                                name={r.credential_name}
                                id={r.credential_id}
                                kind="credential"
                                deleted={r.credential_deleted}
                                disabled={busy || !settings}
                                onClick={() =>
                                  perform(async () => {
                                    const result = await distributionAPI.getCredential(r.credential_id);
                                    setCredential(result.data);
                                  }, false)
                                }
                              />
                            </TableCell>
                            <TableCell>
                              <RecordReference
                                name={r.card_name}
                                id={r.card_id}
                                kind="card"
                                deleted={r.card_deleted}
                                disabled={busy}
                                onClick={() =>
                                  perform(async () => {
                                    const result = await distributionAPI.getCard(r.card_id);
                                    setCard(result.data);
                                  }, false)
                                }
                              />
                            </TableCell>
                            <TableCell>{t(`distribution.plans.${r.kind}`)}</TableCell>
                            <TableCell>{dateText(r.before)}</TableCell>
                            <TableCell>{r.permanent ? t('distribution.states.permanent') : dateText(r.after)}</TableCell>
                            <TableCell>{r.source}</TableCell>
                            <TableCell>{dateText(r.created_at)}</TableCell>
                          </>
                        )}
                      </TableRow>
                    ))}
                    {data.items.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={columns.length + (selectable ? 1 : 0)} sx={{ py: 6, textAlign: 'center' }}>
                          {busy ? <CircularProgress size={24} aria-label={t('distribution.loading')} /> : t('distribution.empty')}
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </TableContainer>
              <TablePagination
                component="div"
                count={data.total}
                page={page}
                onPageChange={(_, value) => setPage(value)}
                rowsPerPage={20}
                rowsPerPageOptions={[20]}
              />
            </Stack>
          )}
        </Box>
      </Stack>
      {issue && (
        <IssueDialog
          kind={resources[tab]}
          subscriptions={subscriptions}
          onClose={() => setIssue(false)}
          onCreated={(rows) => {
            setIssue(false);
            exportRows(rows);
            refresh();
          }}
        />
      )}
      {credential && settings && (
        <CredentialDialog
          credential={credential}
          settings={settings}
          subscriptions={subscriptions}
          onClose={() => setCredential(null)}
          onSaved={refresh}
        />
      )}
      {application && settings && (
        <ReviewDialog
          application={application}
          regionLimit={settings.region_limit ?? 2}
          onClose={() => setApplication(null)}
          onReviewed={() => {
            setApplication(null);
            refresh();
          }}
        />
      )}
      {credentialDeletion && (
        <CredentialDeleteDialog
          credentials={credentialDeletion}
          onClose={() => setCredentialDeletion(null)}
          onDeleted={() => {
            setCredentialDeletion(null);
            setSelected([]);
            setNotice(t('distribution.credentialsDeleted'));
            refresh();
          }}
        />
      )}
      {card && (
        <CardDialog
          card={card}
          onClose={() => setCard(null)}
          onSaved={() => {
            setCard(null);
            setNotice(t('distribution.saved'));
            refresh();
          }}
        />
      )}
      {cardConfirmation && (
        <CardConfirmDialog
          {...cardConfirmation}
          onClose={() => setCardConfirmation(null)}
          onDone={(result) => {
            if (cardConfirmation.reissue) exportRows([result]);
            else setNotice(t('distribution.cardsDeleted'));
            setCardConfirmation(null);
            refresh();
          }}
        />
      )}
      {exportValue && <ExportDialog value={exportValue} onClose={() => setExportValue('')} />}
    </MainCard>
  );
}
