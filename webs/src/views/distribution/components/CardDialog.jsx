import { useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Alert, Button, CircularProgress, MenuItem, Stack, TextField } from '@mui/material';
import { distributionAPI } from 'api/distribution';
import { BusinessDialog, errorText } from './Common';

function localDate(value) {
  if (!value) return '';
  const date = new Date(value);
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

export default function CardDialog({ card, onClose, onSaved }) {
  const { t } = useTranslation();
  const [name, setName] = useState(card.name);
  const [kind, setKind] = useState(card.kind);
  const [deadline, setDeadline] = useState(localDate(card.ends_at));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const readOnly = Boolean(card.deleted);
  const free = card.kind === 'free';
  const locked = free || card.redemption_count > 0 || Boolean(card.bound_credential_id);
  const submit = async (event) => {
    event.preventDefault();
    if (busy || readOnly) return;
    setBusy(true);
    setError('');
    try {
      const data = { name: name.trim() };
      if (!locked) data.kind = kind;
      // Preserve seconds/timezone when the administrator did not edit the field.
      if (!free && deadline !== localDate(card.ends_at)) data.ends_at = deadline ? new Date(deadline).toISOString() : null;
      await distributionAPI.updateCard(card.id, data);
      onSaved();
    } catch (err) {
      setError(errorText(err, t));
    } finally {
      setBusy(false);
    }
  };
  return (
    <BusinessDialog
      title={t(readOnly ? 'distribution.cardDetails' : 'distribution.editCard')}
      open
      onClose={() => !busy && onClose()}
      actions={
        <>
          <Button onClick={onClose} disabled={busy}>
            {t(readOnly ? 'distribution.close' : 'distribution.cancel')}
          </Button>
          {!readOnly && (
            <Button
              type="submit"
              form="card-edit-form"
              variant="contained"
              disabled={busy}
              startIcon={busy ? <CircularProgress size={16} color="inherit" /> : undefined}
            >
              {t(busy ? 'distribution.saving' : 'distribution.save')}
            </Button>
          )}
        </>
      }
    >
      <Stack component="form" id="card-edit-form" onSubmit={submit} spacing={3}>
        {readOnly && <Alert severity="info">{t('distribution.deletedCardReadOnly')}</Alert>}
        <TextField
          autoFocus
          required
          label={t('distribution.name')}
          value={name}
          onChange={(e) => setName(e.target.value)}
          slotProps={{ htmlInput: { maxLength: 100 }, input: { readOnly } }}
        />
        <TextField
          select
          label={t('distribution.plan')}
          value={kind}
          onChange={(e) => setKind(e.target.value)}
          disabled={locked || readOnly}
          helperText={!readOnly && locked ? t(free ? 'distribution.freeCardLocked' : 'distribution.usedCardLocked') : undefined}
        >
          {(free ? ['free'] : ['quarter', 'year', 'permanent']).map((value) => (
            <MenuItem key={value} value={value}>
              {t(`distribution.plans.${value}`)}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          label={t('distribution.cardDeadline')}
          type="datetime-local"
          value={deadline}
          onChange={(e) => setDeadline(e.target.value)}
          disabled={free}
          slotProps={{ inputLabel: { shrink: true }, input: { readOnly } }}
          helperText={!readOnly ? t(free ? 'distribution.freeDeadlineLocked' : 'distribution.cardDeadlineHint') : undefined}
        />
        {error && <Alert severity="error">{error}</Alert>}
      </Stack>
    </BusinessDialog>
  );
}
CardDialog.propTypes = { card: PropTypes.object.isRequired, onClose: PropTypes.func.isRequired, onSaved: PropTypes.func.isRequired };
