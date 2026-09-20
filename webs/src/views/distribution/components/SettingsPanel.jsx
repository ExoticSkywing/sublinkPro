import { useRef, useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Alert, Box, Button, Divider, IconButton, MenuItem, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { IconArrowDown, IconArrowUp, IconPlus, IconTrash } from '@tabler/icons-react';
import { distributionAPI } from 'api/distribution';
import { errorText } from './Common';

export default function SettingsPanel({ initial, subscriptions, onSaved }) {
  const { t } = useTranslation();
  const [form, setForm] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);
  const [notices, setNotices] = useState(() =>
    (initial.expired_messages?.length ? initial.expired_messages : [initial.expired_message, '{portal}']).map((text, id) => ({ id, text }))
  );
  const nextNoticeID = useRef(notices.length);
  const noticeInputs = useRef(new Map());
  const [invalidNotice, setInvalidNotice] = useState(null);
  const editNotices = (next, focusID) => {
    setNotices(next);
    setSaved(false);
    setInvalidNotice(null);
    if (focusID !== undefined) requestAnimationFrame(() => noticeInputs.current.get(focusID)?.focus());
  };
  const field = (name) => ({
    value: form[name] ?? '',
    onChange: (e) => {
      setSaved(false);
      setForm({ ...form, [name]: e.target.value });
    }
  });
  const submit = async (event) => {
    event.preventDefault();
    const invalid = notices.find(({ text }) => !text.trim() || [...text.trim()].length > 200 || /[\p{Cc}\u2028\u2029]/u.test(text));
    if (invalid) {
      setInvalidNotice(invalid.id);
      noticeInputs.current.get(invalid.id)?.focus();
      return;
    }
    setBusy(true);
    setError('');
    setSaved(false);
    try {
      const data = {
        ...form,
        expired_messages: notices.map(({ text }) => text.trim()),
        trial_days: Number(form.trial_days),
        free_days: Number(form.free_days),
        cycle_days: Number(form.cycle_days),
        region_limit: Number(form.region_limit ?? 2),
        fallback_subscription_id: Number(form.fallback_subscription_id)
      };
      await distributionAPI.saveSettings(data);
      onSaved(data);
      setSaved(true);
    } catch (err) {
      setError(errorText(err, t));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Box component="form" onSubmit={submit} sx={{ maxWidth: 640 }}>
      <Stack spacing={3}>
        <Box>
          <Typography variant="h4" component="h2" sx={{ mb: 1 }}>
            {t('distribution.settings')}
          </Typography>
          <Typography color="text.secondary">{t('distribution.settingsHint')}</Typography>
        </Box>
        <TextField label={t('distribution.domain')} {...field('domain')} helperText={t('distribution.domainHint')} />
        <TextField label={t('distribution.portalURL')} {...field('portal_url')} type="url" helperText={t('distribution.portalURLHint')} />
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
          {['trial_days', 'free_days', 'cycle_days'].map((name) => (
            <TextField
              key={name}
              fullWidth
              label={t(`distribution.${name}`)}
              type="number"
              {...field(name)}
              required
              slotProps={{ htmlInput: { min: 1, max: 365 } }}
            />
          ))}
        </Stack>
        <TextField
          label={t('distribution.cycleAnchor')}
          value={form.cycle_anchor?.slice(0, 16) || ''}
          type="datetime-local"
          onChange={(e) => {
            if (e.target.value) setForm({ ...form, cycle_anchor: `${e.target.value}:00Z` });
          }}
          slotProps={{ inputLabel: { shrink: true } }}
          helperText={t('distribution.cycleAnchorHint')}
          required
        />
        <Divider />
        <TextField
          select
          label={t('distribution.fallbackSubscription')}
          {...field('fallback_subscription_id')}
          helperText={t('distribution.fallbackHint')}
        >
          <MenuItem value={0}>{t('distribution.noticeOnly')}</MenuItem>
          {subscriptions.map((s) => (
            <MenuItem key={s.ID} value={s.ID}>
              {s.Name}
            </MenuItem>
          ))}
        </TextField>
        <Stack component="section" aria-labelledby="expiry-notices-heading" spacing={2}>
          <Box>
            <Typography id="expiry-notices-heading" component="h3" variant="h5">
              {t('distribution.expiredNotices', { count: notices.length })}
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              {t('distribution.expiredNoticesHint')}
            </Typography>
          </Box>
          {notices.map((notice, index) => (
            <Stack key={notice.id} direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'flex-start' }}>
              <TextField
                fullWidth
                label={t('distribution.noticeNode', { number: index + 1 })}
                value={notice.text}
                inputRef={(element) => {
                  if (element) noticeInputs.current.set(notice.id, element);
                  else noticeInputs.current.delete(notice.id);
                }}
                onChange={(event) => editNotices(notices.map((row) => (row.id === notice.id ? { ...row, text: event.target.value } : row)))}
                disabled={busy}
                required
                error={invalidNotice === notice.id}
                helperText={invalidNotice === notice.id ? t('distribution.noticeValidation') : undefined}
              />
              <Stack direction="row" spacing={1} sx={{ alignSelf: { xs: 'flex-end', sm: 'auto' } }}>
                {[
                  ['moveNoticeUp', IconArrowUp, index === 0, -1],
                  ['moveNoticeDown', IconArrowDown, index === notices.length - 1, 1],
                  ['removeNotice', IconTrash, notices.length === 1, 0]
                ].map(([key, ActionIcon, disabled, offset]) => (
                  <Tooltip key={key} title={t(`distribution.${key}`, { number: index + 1 })}>
                    <span>
                      <IconButton
                        aria-label={t(`distribution.${key}`, { number: index + 1 })}
                        disabled={busy || disabled}
                        sx={{ width: { xs: 44, sm: 40 }, height: { xs: 44, sm: 40 }, color: 'text.secondary' }}
                        onClick={() => {
                          const next = [...notices];
                          if (offset) {
                            [next[index], next[index + offset]] = [next[index + offset], next[index]];
                            editNotices(next, notice.id);
                          } else {
                            next.splice(index, 1);
                            editNotices(next, next[Math.min(index, next.length - 1)].id);
                          }
                        }}
                      >
                        <ActionIcon size={20} aria-hidden="true" />
                      </IconButton>
                    </span>
                  </Tooltip>
                ))}
              </Stack>
            </Stack>
          ))}
          <Button
            variant="outlined"
            startIcon={<IconPlus size={16} aria-hidden="true" />}
            disabled={busy || notices.length >= 10}
            sx={{ alignSelf: 'flex-start' }}
            onClick={() => {
              const id = nextNoticeID.current++;
              editNotices([...notices, { id, text: '' }], id);
            }}
          >
            {t('distribution.addNotice')}
          </Button>
        </Stack>
        <TextField
          select
          label={t('distribution.regionLimit')}
          {...field('region_limit')}
          value={form.region_limit ?? 2}
          disabled={busy}
          helperText={t('distribution.regionLimitHint')}
        >
          {[1, 2].map((limit) => (
            <MenuItem key={limit} value={limit}>
              {t('distribution.regionLimitOption', { count: limit })}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          label={t('distribution.allowedUA')}
          {...field('allowed_ua')}
          multiline
          minRows={3}
          required
          helperText={t('distribution.uaHint')}
          slotProps={{ htmlInput: { maxLength: 2000 } }}
        />
        <Alert severity="info">{t('distribution.cityPolicyHint', { limit: form.region_limit ?? 2 })}</Alert>
        {error && <Alert severity="error">{error}</Alert>}
        {saved && (
          <Alert severity="success" role="status">
            {t('distribution.saved')}
          </Alert>
        )}
        <Button type="submit" variant="contained" disabled={busy} sx={{ alignSelf: 'flex-start' }}>
          {t(busy ? 'distribution.saving' : 'distribution.save')}
        </Button>
      </Stack>
    </Box>
  );
}
SettingsPanel.propTypes = {
  initial: PropTypes.object.isRequired,
  subscriptions: PropTypes.array.isRequired,
  onSaved: PropTypes.func.isRequired
};
