import { useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Typography } from '@mui/material';
import { distributionAPI } from 'api/distribution';
import { BusinessDialog, cardDeleteButtonSx, errorText } from './Common';

export default function CredentialDeleteDialog({ credentials, onClose, onDeleted }) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const confirm = async () => {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      await distributionAPI.deleteCredentials(credentials.map((row) => row.id));
      onDeleted();
    } catch (err) {
      setError(errorText(err, t));
    } finally {
      setBusy(false);
    }
  };
  return (
    <BusinessDialog
      title={t('distribution.deleteCredentialsTitle', { count: credentials.length })}
      open
      onClose={() => !busy && onClose()}
      actions={
        <>
          <Button autoFocus onClick={onClose} disabled={busy}>
            {t('distribution.cancel')}
          </Button>
          <Button variant="contained" color="error" sx={cardDeleteButtonSx} onClick={confirm} loading={busy}>
            {t('distribution.confirmDelete')}
          </Button>
        </>
      }
    >
      <Typography>{t('distribution.deleteCredentialsHint')}</Typography>
      <Typography sx={{ overflowWrap: 'anywhere' }}>{credentials.map((row) => `${row.name} (#${row.id})`).join('、')}</Typography>
      {error && <Alert severity="error">{error}</Alert>}
    </BusinessDialog>
  );
}
CredentialDeleteDialog.propTypes = {
  credentials: PropTypes.array.isRequired,
  onClose: PropTypes.func.isRequired,
  onDeleted: PropTypes.func.isRequired
};
