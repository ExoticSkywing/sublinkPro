import { useEffect, useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Alert, Button, MenuItem, TextField, Typography } from '@mui/material';
import { distributionAPI } from 'api/distribution';
import { BusinessDialog, dateText, errorText } from './Common';

export default function ReviewDialog({ application, regionLimit, onClose, onReviewed }) {
  const { t } = useTranslation();
  const [credential, setCredential] = useState(null);
  const [replace, setReplace] = useState('');
  const [reply, setReply] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    distributionAPI
      .getCredential(application.credential_id)
      .then((r) => setCredential(r.data))
      .catch((err) => setError(errorText(err, t)));
  }, [application.credential_id, t]);
  const review = async (approve) => {
    setBusy(true);
    setError('');
    try {
      await distributionAPI.review(application.id, { approve, replace_region_id: Number(replace), reply });
      onReviewed();
    } catch (err) {
      setError(errorText(err, t));
    } finally {
      setBusy(false);
    }
  };
  return (
    <BusinessDialog
      title={t('distribution.reviewApplication')}
      open
      onClose={onClose}
      actions={
        <>
          <Button onClick={onClose}>{t('distribution.cancel')}</Button>
          <Button color="error" onClick={() => review(false)} disabled={busy || !reply.trim()}>
            {t('distribution.reject')}
          </Button>
          <Button
            variant="contained"
            onClick={() => review(true)}
            disabled={busy || !credential || (credential.regions.length >= regionLimit && !replace)}
          >
            {t('distribution.approveReplace')}
          </Button>
        </>
      }
    >
      <Typography fontWeight={600}>
        #{application.credential_id} · {credential?.name || '…'}
      </Typography>
      <Typography>
        {t('distribution.targetCity')}: {application.province} · {application.city}
      </Typography>
      <Typography color="text.secondary">{dateText(application.created_at)}</Typography>
      <TextField label={t('distribution.reason')} value={application.reason} multiline slotProps={{ input: { readOnly: true } }} />
      {application.contact && (
        <Typography sx={{ overflowWrap: 'anywhere' }}>
          {t('distribution.contact')}: {application.contact}
        </Typography>
      )}
      <TextField
        select
        label={t('distribution.replaceCity')}
        value={replace}
        onChange={(e) => setReplace(e.target.value)}
        helperText={t('distribution.replaceHint', { limit: regionLimit })}
      >
        {(credential?.regions || []).map((r) => (
          <MenuItem key={r.id} value={r.id}>
            {r.province} · {r.city}
          </MenuItem>
        ))}
      </TextField>
      <TextField
        label={t('distribution.reviewReply')}
        value={reply}
        onChange={(e) => setReply(e.target.value)}
        multiline
        minRows={2}
        helperText={t('distribution.replyHint')}
        slotProps={{ htmlInput: { maxLength: 1000 } }}
      />
      <Alert severity="info">{t('distribution.reviewHint')}</Alert>
      {error && <Alert severity="error">{error}</Alert>}
    </BusinessDialog>
  );
}
ReviewDialog.propTypes = {
  application: PropTypes.object.isRequired,
  regionLimit: PropTypes.number.isRequired,
  onClose: PropTypes.func.isRequired,
  onReviewed: PropTypes.func.isRequired
};
