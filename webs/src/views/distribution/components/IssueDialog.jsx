import { useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Alert, Button, MenuItem, Stack, TextField } from '@mui/material';
import { distributionAPI } from 'api/distribution';
import { BusinessDialog, errorText } from './Common';

export default function IssueDialog({ kind, subscriptions, onClose, onCreated }) {
  const { t } = useTranslation();
  const [form, setForm] = useState({
    name: '',
    batch: '',
    count: 1,
    subscription_id: subscriptions[0]?.ID || '',
    kind: 'quarter',
    ends_at: ''
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const count = Number(form.count);
  const isBatch = Number.isInteger(count) && count > 1 && count <= 100;
  const field = (name) => ({ value: form[name], onChange: (e) => setForm({ ...form, [name]: e.target.value }) });
  const submit = async (event) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      const payload = {
        ...form,
        count: Number(form.count),
        subscription_id: Number(form.subscription_id),
        ends_at: form.ends_at ? new Date(form.ends_at).toISOString() : null
      };
      const result = kind === 'credentials' ? await distributionAPI.issue(payload) : await distributionAPI.createCards(payload);
      onCreated(result.data);
    } catch (err) {
      setError(errorText(err, t));
    } finally {
      setBusy(false);
    }
  };
  return (
    <BusinessDialog
      title={t(kind === 'credentials' ? 'distribution.issueCredentials' : 'distribution.issueCards')}
      open
      onClose={onClose}
      actions={
        <>
          <Button onClick={onClose} disabled={busy}>
            {t('distribution.cancel')}
          </Button>
          <Button type="submit" form="issue-form" variant="contained" disabled={busy}>
            {t('distribution.generate')}
          </Button>
        </>
      }
    >
      <Stack component="form" id="issue-form" onSubmit={submit} spacing={3} useFlexGap>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={3} useFlexGap>
          <TextField
            label={t(isBatch ? 'distribution.namePrefix' : 'distribution.name')}
            {...field('name')}
            required
            fullWidth
            slotProps={{ htmlInput: { maxLength: 80 }, formHelperText: { sx: { overflowWrap: 'anywhere' } } }}
            helperText={
              isBatch && form.name.trim()
                ? t('distribution.batchNamePreview', {
                    first: `${form.name}-001`,
                    last: `${form.name}-${String(count).padStart(3, '0')}`
                  })
                : undefined
            }
          />
          <TextField
            label={t('distribution.count')}
            type="number"
            {...field('count')}
            required
            sx={{ width: { xs: '100%', sm: 128 }, flexShrink: 0 }}
            slotProps={{ htmlInput: { min: 1, max: 100 } }}
          />
        </Stack>
        {kind === 'credentials' ? (
          <TextField
            select
            label={t('distribution.resourceSubscription')}
            {...field('subscription_id')}
            required
            helperText={t('distribution.resourceHint')}
          >
            {subscriptions.map((s) => (
              <MenuItem key={s.ID} value={s.ID}>
                {s.Name}
              </MenuItem>
            ))}
          </TextField>
        ) : (
          <>
            <TextField select label={t('distribution.plan')} {...field('kind')}>
              {['quarter', 'year', 'permanent'].map((item) => (
                <MenuItem key={item} value={item}>
                  {t(`distribution.plans.${item}`)}
                </MenuItem>
              ))}
            </TextField>
            <TextField
              label={t('distribution.cardDeadline')}
              type="datetime-local"
              {...field('ends_at')}
              slotProps={{ inputLabel: { shrink: true } }}
              helperText={t('distribution.cardDeadlineHint')}
            />
          </>
        )}
        <TextField
          label={t('distribution.batch')}
          {...field('batch')}
          helperText={t('distribution.batchHint')}
          slotProps={{ htmlInput: { maxLength: 100 } }}
        />
        {error && <Alert severity="error">{error}</Alert>}
      </Stack>
    </BusinessDialog>
  );
}
IssueDialog.propTypes = {
  kind: PropTypes.string.isRequired,
  subscriptions: PropTypes.array.isRequired,
  onClose: PropTypes.func.isRequired,
  onCreated: PropTypes.func.isRequired
};
