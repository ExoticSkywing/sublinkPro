import { useEffect, useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useColorScheme } from '@mui/material/styles';
import { Alert, Box, Button, Container, Divider, MenuItem, Paper, Stack, Tab, Tabs, TextField, Typography } from '@mui/material';
import { distributionPublic } from 'api/distribution';
import { dateText, credentialState, errorText, StatusChip, distributionSurfaceSx } from './components/Common';

export default function DistributionPortal() {
  const { t } = useTranslation();
  const { mode, systemMode, setMode } = useColorScheme();
  const [tab, setTab] = useState(0);
  const [link, setLink] = useState('');
  const [code, setCode] = useState('');
  const [visit, setVisit] = useState('');
  const [reason, setReason] = useState('');
  const [contact, setContact] = useState('');
  const [status, setStatus] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState(null);
  const statusID = status?.id;
  useEffect(() => {
    if (!statusID || busy) return;
    let active = true;
    let inFlight = false;
    let lastRefresh = 0;
    const refresh = async () => {
      if (document.visibilityState !== 'visible' || inFlight || Date.now() - lastRefresh < 1000) return;
      inFlight = true;
      lastRefresh = Date.now();
      try {
        const result = await distributionPublic('status', { link });
        if (active) setStatus(result);
      } catch {
        if (active) {
          setStatus(null);
          setError(t('distribution.statusLookupFailed'));
        }
      } finally {
        inFlight = false;
      }
    };
    const timer = setInterval(refresh, 60000);
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    return () => {
      active = false;
      clearInterval(timer);
      window.removeEventListener('focus', refresh);
      document.removeEventListener('visibilitychange', refresh);
    };
  }, [statusID, link, busy, t]);
  const lookup = async () => {
    setStatus(null);
    const result = await distributionPublic('status', { link });
    setStatus(result);
    return result;
  };
  const perform = async (action) => {
    setBusy(true);
    setError('');
    setMessage(null);
    try {
      await action();
    } catch (err) {
      setError(errorText(err, t));
    } finally {
      setBusy(false);
    }
  };
  const submit = (event) => {
    event.preventDefault();
    perform(async () => {
      if (tab === 0) {
        const result = await distributionPublic('redeem', { link, code });
        setCode('');
        setMessage({
          severity: result.already_redeemed ? 'info' : 'success',
          key: result.already_redeemed
            ? result.kind === 'free'
              ? 'distribution.freeAlreadyRedeemed'
              : 'distribution.cardAlreadyRedeemed'
            : 'distribution.redeemSuccess',
          time: dateText(result.created_at)
        });
      } else {
        await distributionPublic('region-requests', { link, visit_id: Number(visit), reason, contact });
        setReason('');
        setMessage({ severity: 'success', key: 'distribution.applySuccess' });
      }
      try {
        await lookup();
      } catch {
        // The mutation has completed. Do not present a status-fetch failure as
        // a failed redemption, or leave a stale expiry displayed underneath.
        setError(t('distribution.statusRefreshFailed'));
      }
    });
  };
  return (
    <Box
      component="main"
      sx={{ ...distributionSurfaceSx, minHeight: '100dvh', bgcolor: 'background.default', py: { xs: 3, sm: 6 }, px: 2 }}
    >
      <Container maxWidth="sm" disableGutters>
        <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 3 }}>
          <Typography component="span" variant="h4">
            {t('distribution.portalBrand')}
          </Typography>
          <Button size="small" onClick={() => setMode((mode === 'system' ? systemMode : mode) === 'dark' ? 'light' : 'dark')}>
            {t('distribution.toggleTheme')}
          </Button>
        </Stack>
        <Paper variant="outlined" sx={{ p: { xs: 2, sm: 4 }, borderRadius: 2 }}>
          <Stack spacing={3}>
            <Box>
              <Typography component="h1" variant="h2" sx={{ fontSize: 24, mb: 1 }}>
                {t('distribution.portalTitle')}
              </Typography>
              <Typography color="text.secondary">{t('distribution.portalDescription')}</Typography>
            </Box>
            <Tabs
              value={tab}
              onChange={(_, value) => {
                setTab(value);
                setError('');
                setMessage(null);
              }}
              variant="fullWidth"
              aria-label={t('distribution.portalActions')}
            >
              <Tab disabled={busy} id="portal-tab-0" aria-controls="portal-panel" label={t('distribution.redeem')} />
              <Tab disabled={busy} id="portal-tab-1" aria-controls="portal-panel" label={t('distribution.regionApplication')} />
            </Tabs>
            <Box component="form" onSubmit={submit} id="portal-panel" role="tabpanel" aria-labelledby={`portal-tab-${tab}`}>
              <Stack spacing={2}>
                <TextField
                  label={t('distribution.subscriptionLink')}
                  value={link}
                  disabled={busy}
                  onChange={(e) => {
                    setLink(e.target.value);
                    setStatus(null);
                    setVisit('');
                    setMessage(null);
                    setError('');
                  }}
                  required
                  fullWidth
                  autoComplete="off"
                  slotProps={{ htmlInput: { maxLength: 2048, spellCheck: false } }}
                  helperText={t('distribution.linkHint')}
                />
                <Button type="button" variant="outlined" disabled={busy || !link.trim()} onClick={() => perform(lookup)}>
                  {t('distribution.queryStatus')}
                </Button>
                {tab === 0 ? (
                  <TextField
                    label={t('distribution.cardCode')}
                    value={code}
                    onChange={(e) => {
                      setCode(e.target.value);
                      setMessage(null);
                      setError('');
                    }}
                    disabled={busy}
                    required
                    fullWidth
                    autoComplete="off"
                    slotProps={{ htmlInput: { maxLength: 128, spellCheck: false } }}
                  />
                ) : (
                  <>
                    <Alert severity="info">{t('distribution.regionHint')}</Alert>
                    <TextField
                      select
                      label={t('distribution.targetCity')}
                      value={visit}
                      onChange={(e) => setVisit(e.target.value)}
                      required
                      fullWidth
                      disabled={!status?.candidates?.length}
                      helperText={!status?.candidates?.length ? t('distribution.noCandidate') : t('distribution.candidateHint')}
                    >
                      {(status?.candidates || []).map((item) => (
                        <MenuItem key={item.id} value={item.id}>
                          {item.province} · {item.city}
                        </MenuItem>
                      ))}
                    </TextField>
                    <TextField
                      label={t('distribution.reason')}
                      value={reason}
                      onChange={(e) => setReason(e.target.value)}
                      required
                      multiline
                      minRows={3}
                      slotProps={{ htmlInput: { minLength: 3, maxLength: 1000 } }}
                    />
                    <TextField
                      label={t('distribution.contactOptional')}
                      value={contact}
                      onChange={(e) => setContact(e.target.value)}
                      slotProps={{ htmlInput: { maxLength: 200 } }}
                    />
                  </>
                )}
                {error && <Alert severity="error">{error}</Alert>}
                <Box role="status" aria-live="polite" aria-atomic="true">
                  {message && (
                    <Alert severity={message.severity} role="presentation">
                      {t(message.key, { time: message.time })}
                    </Alert>
                  )}
                </Box>
                <Button type="submit" size="large" variant="contained" disabled={busy || (tab === 1 && !visit)}>
                  {busy ? t('distribution.processing') : t(tab === 0 ? 'distribution.redeem' : 'distribution.submitApplication')}
                </Button>
              </Stack>
            </Box>
            {status && (
              <Box sx={{ bgcolor: 'background.default', border: '1px solid', borderColor: 'divider', p: 2, borderRadius: 1 }}>
                <Stack spacing={2}>
                  <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Typography fontWeight={600}>{t('distribution.yourSubscription')}</Typography>
                    <StatusChip value={credentialState(status)} />
                  </Stack>
                  <Typography>
                    {t('distribution.expiresAt')}: {status.permanent ? t('distribution.states.permanent') : dateText(status.expires_at)}
                  </Typography>
                  <Typography color="text.secondary">
                    {t('distribution.allowedCities')} ({status.regions.length}/{status.region_limit ?? '—'}):{' '}
                    {status.regions.map((r) => `${r.province}·${r.city}`).join(' / ') || t('distribution.notActivatedHint')}
                  </Typography>
                  {status.requests.length > 0 && (
                    <>
                      <Divider />
                      <Typography fontWeight={600}>{t('distribution.applicationHistory')}</Typography>
                      {status.requests.map((r) => (
                        <Box key={r.id}>
                          <Stack direction="row" spacing={1} alignItems="center">
                            <Typography>
                              {r.province} · {r.city}
                            </Typography>
                            <StatusChip value={r.status} />
                          </Stack>
                          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                            {r.reply || dateText(r.created_at)}
                          </Typography>
                        </Box>
                      ))}
                    </>
                  )}
                </Stack>
              </Box>
            )}
          </Stack>
        </Paper>
        <Stack direction="row" justifyContent="center" sx={{ mt: 3 }}>
          <Button component={RouterLink} to="/admin/login" size="small">
            {t('distribution.adminEntry')}
          </Button>
        </Stack>
      </Container>
    </Box>
  );
}
