import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Button, Stack, Typography } from '@mui/material';

export default function RecordReference({ name, id, kind, deleted = false, onClick, disabled }) {
  const { t } = useTranslation();
  const label = name || t(kind === 'card' ? 'distribution.cardFallback' : 'distribution.credentialFallback', { id });
  return (
    <Stack spacing={0.5} sx={{ minWidth: 128, maxWidth: 256 }}>
      <Button
        onClick={onClick}
        disabled={disabled}
        aria-label={t(kind === 'card' ? 'distribution.viewCard' : 'distribution.viewCredential', { name: label, id })}
        sx={{
          minWidth: 0,
          minHeight: { xs: 44, sm: 40 },
          px: 0.5,
          justifyContent: 'flex-start',
          textAlign: 'start',
          fontWeight: 600,
          textTransform: 'none',
          textDecoration: 'underline',
          overflowWrap: 'anywhere',
          '&.Mui-focusVisible': { outline: '2px solid', outlineColor: 'text.primary', outlineOffset: 2 }
        }}
      >
        {label}
      </Button>
      <Typography variant="caption" color="text.secondary">
        #{id}
        {deleted ? ` · ${t('distribution.deleted')}` : ''}
      </Typography>
    </Stack>
  );
}
RecordReference.propTypes = {
  name: PropTypes.string,
  id: PropTypes.number.isRequired,
  kind: PropTypes.oneOf(['credential', 'card']).isRequired,
  deleted: PropTypes.bool,
  onClick: PropTypes.func.isRequired,
  disabled: PropTypes.bool
};
