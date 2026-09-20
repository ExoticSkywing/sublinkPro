import { useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Alert, Button, CircularProgress, Typography } from '@mui/material';
import { distributionAPI } from 'api/distribution';
import { BusinessDialog, cardDeleteButtonSx, errorText } from './Common';

export default function CardConfirmDialog({ cards, reissue = false, onClose, onDone }) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const confirm = async () => {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      const result = reissue ? await distributionAPI.reissueFreeCard() : await distributionAPI.deleteCards(cards.map((r) => r.id));
      onDone(result.data);
    } catch (err) {
      setError(errorText(err, t));
    } finally {
      setBusy(false);
    }
  };
  return (
    <BusinessDialog
      title={t(reissue ? 'distribution.reissueFreeCard' : 'distribution.deleteCardsTitle', { count: cards.length })}
      open
      onClose={() => !busy && onClose()}
      actions={
        <>
          <Button autoFocus onClick={onClose} disabled={busy}>
            {t('distribution.cancel')}
          </Button>
          <Button
            variant="contained"
            color={reissue ? 'primary' : 'error'}
            sx={reissue ? undefined : cardDeleteButtonSx}
            onClick={confirm}
            disabled={busy}
            startIcon={busy ? <CircularProgress size={16} color="inherit" /> : undefined}
          >
            {t(busy ? 'distribution.processing' : reissue ? 'distribution.confirmReissue' : 'distribution.confirmDelete')}
          </Button>
        </>
      }
    >
      <Typography>{t(reissue ? 'distribution.reissueFreeHint' : 'distribution.deleteCardsHint')}</Typography>
      {!reissue && <Typography sx={{ overflowWrap: 'anywhere' }}>{cards.map((r) => `${r.name} (#${r.id})`).join('、')}</Typography>}
      {!reissue && cards.some((r) => r.kind === 'free') && <Alert severity="info">{t('distribution.deleteFreeHint')}</Alert>}
      {error && <Alert severity="error">{error}</Alert>}
    </BusinessDialog>
  );
}
CardConfirmDialog.propTypes = {
  cards: PropTypes.array.isRequired,
  reissue: PropTypes.bool,
  onClose: PropTypes.func.isRequired,
  onDone: PropTypes.func.isRequired
};
