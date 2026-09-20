import { useEffect, useRef, useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Alert, Box, Button, Checkbox, Divider, FormControlLabel, MenuItem, Stack, TextField, Typography } from '@mui/material';
import { distributionAPI } from 'api/distribution';
import { BusinessDialog, credentialState, dateText, errorText, StatusChip, subscriptionURL } from './Common';

export default function CredentialDialog({ credential, settings, subscriptions, onClose, onSaved }) {
  const { t } = useTranslation();
  const [row, setRow] = useState(credential);
  const [visits, setVisits] = useState([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [copyStatus, setCopyStatus] = useState('');
  const linkRef = useRef(null);
  const [name, setName] = useState(credential.name);
  const [subscriptionId, setSubscriptionId] = useState(credential.subscription_id);
  const [state, setState] = useState(credential.status);
  const [permanent, setPermanent] = useState(credential.permanent);
  const [expires, setExpires] = useState(credential.expires_at?.slice(0, 16) || '');
  const [rotate, setRotate] = useState(false);
  const readOnly = busy || row.status === 'revoked';
  const copyLink = async () => {
    setCopyStatus('');
    try {
      await navigator.clipboard.writeText(subscriptionURL(row.token, settings));
      setCopyStatus('success');
    } catch {
      linkRef.current?.focus();
      linkRef.current?.select();
      setCopyStatus('error');
    }
  };
  useEffect(() => {
    distributionAPI
      .list('visits', { credential_id: credential.id, size: 20 })
      .then((r) => setVisits(r.data.items))
      .catch((err) => setError(errorText(err, t)));
  }, [credential.id, t]);
  const save = async () => {
    setBusy(true);
    setError('');
    setSaved(false);
    try {
      const data = { name, status: state, rotate };
      if (Number(subscriptionId) !== row.subscription_id) data.subscription_id = Number(subscriptionId);
      if (row.activated_at && (permanent !== row.permanent || expires !== (row.expires_at?.slice(0, 16) || ''))) {
        data.permanent = permanent;
        if (!permanent && expires) data.expires_at = `${expires}:00Z`;
      }
      const result = await distributionAPI.updateCredential(row.id, data);
      setRow(result.data);
      setName(result.data.name);
      setState(result.data.status);
      setSubscriptionId(result.data.subscription_id);
      setExpires(result.data.expires_at?.slice(0, 16) || '');
      setPermanent(result.data.permanent);
      setRotate(false);
      setCopyStatus('');
      setSaved(true);
      onSaved();
    } catch (err) {
      setError(errorText(err, t));
    } finally {
      setBusy(false);
    }
  };
  return (
    <BusinessDialog
      title={`${t('distribution.credential')} #${row.id}`}
      open
      onClose={busy ? undefined : onClose}
      actions={
        <Stack spacing={1} sx={{ width: '100%' }}>
          <Box role="status" aria-live="polite" aria-atomic="true">
            {saved && (
              <Alert severity="success" role="presentation">
                {t('distribution.saved')}
              </Alert>
            )}
          </Box>
          {error && <Alert severity="error">{error}</Alert>}
          <Stack direction="row" spacing={1} justifyContent="flex-end">
            <Button onClick={onClose} disabled={busy}>
              {t('distribution.close')}
            </Button>
            {row.status !== 'revoked' && (
              <Button variant="contained" loading={busy} onClick={save}>
                {t(busy ? 'distribution.saving' : 'distribution.save')}
              </Button>
            )}
          </Stack>
        </Stack>
      }
    >
      <Stack direction="row" spacing={1}>
        <StatusChip value={credentialState(row)} />
        <Typography color="text.secondary">{t(`distribution.plans.${row.plan}`)}</Typography>
      </Stack>
      {row.status === 'revoked' ? (
        <Alert severity="info">{t('distribution.deletedCredentialReadOnly')}</Alert>
      ) : (
        <Stack spacing={1}>
          <TextField
            inputRef={linkRef}
            label={t('distribution.subscriptionLink')}
            value={subscriptionURL(row.token, settings)}
            multiline
            error={copyStatus === 'error'}
            helperText={copyStatus === 'error' ? t('common.copyFailedManual') : undefined}
            slotProps={{ input: { readOnly: true } }}
          />
          <Button
            variant="outlined"
            onClick={copyLink}
            disabled={busy}
            aria-live="polite"
            aria-atomic="true"
            sx={{ alignSelf: 'flex-start' }}
          >
            {t(copyStatus === 'success' ? 'distribution.copied' : 'distribution.copyLink')}
          </Button>
        </Stack>
      )}
      <Stack spacing={2} onChange={() => setSaved(false)}>
        <TextField
          label={t('distribution.name')}
          value={name}
          disabled={readOnly}
          onChange={(e) => setName(e.target.value)}
          slotProps={{ htmlInput: { maxLength: 100 } }}
        />
        <TextField label={t('distribution.batchId')} value={row.batch || '—'} multiline slotProps={{ input: { readOnly: true } }} />
        <TextField
          select
          label={t('distribution.resourceSubscription')}
          value={subscriptionId}
          onChange={(e) => {
            setSubscriptionId(e.target.value);
            setSaved(false);
          }}
          disabled={readOnly}
          helperText={Number(subscriptionId) !== row.subscription_id ? t('distribution.resourceChangeHint') : undefined}
        >
          {!subscriptions.some((s) => s.ID === row.subscription_id) && (
            <MenuItem value={row.subscription_id}>#{row.subscription_id}</MenuItem>
          )}
          {subscriptions.map((s) => (
            <MenuItem key={s.ID} value={s.ID}>
              {s.Name}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          label={t('distribution.status')}
          value={state}
          onChange={(e) => {
            setState(e.target.value);
            setSaved(false);
          }}
          disabled={readOnly}
        >
          {(row.status === 'revoked' ? ['revoked'] : ['enabled', 'disabled']).map((s) => (
            <MenuItem value={s} key={s}>
              {t(`distribution.states.${s}`)}
            </MenuItem>
          ))}
        </TextField>
        <Typography color="text.secondary">
          {t('distribution.activatedAt')}: {dateText(row.activated_at)}
        </Typography>
        {row.activated_at && (
          <>
            <FormControlLabel
              control={<Checkbox checked={permanent} onChange={(e) => setPermanent(e.target.checked)} disabled={readOnly} />}
              label={t('distribution.states.permanent')}
            />
            <TextField
              type="datetime-local"
              label={`${t('distribution.expiresAt')} (UTC)`}
              value={expires}
              onChange={(e) => setExpires(e.target.value)}
              disabled={readOnly || permanent}
              slotProps={{ inputLabel: { shrink: true } }}
            />
          </>
        )}
        <FormControlLabel
          control={<Checkbox checked={rotate} onChange={(e) => setRotate(e.target.checked)} disabled={readOnly} />}
          label={t('distribution.rotateToken')}
        />
        {rotate && <Alert severity="warning">{t('distribution.rotateHint')}</Alert>}
      </Stack>
      <Typography fontWeight={600}>
        {t('distribution.allowedCities')} ({row.regions?.length || 0}/{settings.region_limit ?? 2})
      </Typography>
      <Typography>{row.regions?.map((r) => `${r.province} · ${r.city}`).join(' / ') || '—'}</Typography>
      <Divider />
      <Typography component="h3" variant="h4">
        {t('distribution.recentVisits')}
      </Typography>
      {visits.length === 0 ? (
        <Typography color="text.secondary">{t('distribution.empty')}</Typography>
      ) : (
        visits.map((v) => (
          <Box key={v.id} sx={{ p: 2, border: '1px solid', borderColor: 'divider', bgcolor: 'background.default', borderRadius: 1 }}>
            <Stack direction="row" justifyContent="space-between" spacing={1}>
              <Typography fontWeight={600}>
                {v.province} {v.city || v.country || '—'}
              </Typography>
              <Typography variant="body2">{t(`distribution.results.${v.result}`, { defaultValue: v.result })}</Typography>
            </Stack>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1, overflowWrap: 'anywhere' }}>
              {v.ip} · {v.ua}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {dateText(v.created_at)}
            </Typography>
          </Box>
        ))
      )}
    </BusinessDialog>
  );
}
CredentialDialog.propTypes = {
  credential: PropTypes.object.isRequired,
  settings: PropTypes.object.isRequired,
  subscriptions: PropTypes.array.isRequired,
  onClose: PropTypes.func.isRequired,
  onSaved: PropTypes.func.isRequired
};
