import { useRef, useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Chip, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, useMediaQuery } from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { IconAlertTriangle, IconBan, IconCircleCheck, IconCircleX, IconClock, IconInfinity } from '@tabler/icons-react';
import { withAlpha } from 'utils/colorUtils';

// Keep status text readable with the upstream palette's very bright greens and
// yellows. Scope these roles to this feature; do not alter the upstream theme.
export const distributionSurfaceSx = {
  '& .MuiAlert-message, & .MuiChip-label': { color: 'text.primary' },
  '& .MuiAlert-icon': { color: 'text.secondary' },
  '& .MuiButton-textPrimary, & .MuiButton-outlinedPrimary, & .MuiTab-root.Mui-selected': { color: 'text.primary' },
  '& .MuiButton-containedPrimary': {
    bgcolor: 'secondary.main',
    color: 'secondary.contrastText',
    '&:hover': { bgcolor: 'secondary.dark' },
    '&.Mui-disabled': { bgcolor: 'action.disabledBackground', color: 'action.disabled' }
  }
};

// The upstream error.main is too light for small text on white surfaces.
export const cardDeleteButtonSx = (theme) => ({
  color: 'error.dark',
  ...theme.applyStyles('dark', { color: 'error.light' }),
  '&.MuiButton-containedError': {
    bgcolor: 'error.dark',
    color: 'error.contrastText',
    '&:hover': { bgcolor: 'error.dark' },
    '&.Mui-disabled': { bgcolor: 'action.disabledBackground', color: 'action.disabled' }
  }
});

export function errorText(error, t) {
  const key = error?.i18nKey || error?.response?.data?.i18nKey || 'distribution.errors.server_error';
  return t(key);
}
export function dateText(value) {
  return value ? new Date(value).toLocaleString() : '—';
}
export function credentialState(row) {
  if (row.status !== 'enabled') return row.status;
  if (!row.activated_at) return 'unactivated';
  if (row.permanent) return 'permanent';
  return row.expires_at && new Date(row.expires_at) > new Date() ? 'active' : 'expired';
}
export function subscriptionURL(token, cfg) {
  const origin = cfg?.domain ? `https://${cfg.domain}` : window.location.origin;
  return `${origin}/d/${token}`;
}
export function StatusChip({ value }) {
  const { t } = useTranslation();
  const colors = {
    active: 'success',
    approved: 'success',
    enabled: 'success',
    permanent: 'success',
    expired: 'warning',
    pending: 'warning',
    disabled: 'default',
    revoked: 'error',
    rejected: 'error'
  };
  const icons = {
    active: IconCircleCheck,
    approved: IconCircleCheck,
    enabled: IconCircleCheck,
    permanent: IconInfinity,
    expired: IconAlertTriangle,
    pending: IconClock,
    unactivated: IconClock,
    disabled: IconBan,
    revoked: IconCircleX,
    rejected: IconCircleX
  };
  const StateIcon = icons[value] || IconClock;
  return (
    <Chip
      size="small"
      icon={<StateIcon size={16} stroke={2} aria-hidden="true" />}
      label={t(`distribution.states.${value}`, { defaultValue: value })}
      sx={(theme) => {
        const palette = theme.vars?.palette || theme.palette;
        const accent = palette[colors[value]]?.main || palette.text.secondary;
        return {
          height: 32,
          borderRadius: 1,
          fontWeight: 600,
          color: 'text.primary',
          bgcolor: withAlpha(accent, 0.16),
          border: '1px solid',
          borderColor: withAlpha(accent, 0.4),
          '& .MuiChip-icon': { color: 'inherit', ml: 1 },
          '& .MuiChip-label': { px: 1 }
        };
      }}
    />
  );
}
StatusChip.propTypes = { value: PropTypes.string.isRequired };

export function BusinessDialog({ title, open, onClose, children, actions }) {
  const theme = useTheme();
  const mobile = useMediaQuery(theme.breakpoints.down('sm'));
  return (
    <Dialog
      open={open}
      onClose={onClose}
      fullWidth
      maxWidth="sm"
      fullScreen={mobile}
      aria-labelledby="business-dialog-title"
      sx={distributionSurfaceSx}
    >
      <DialogTitle id="business-dialog-title">{title}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 2 }}>
          {children}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ p: 2, gap: 1 }}>{actions}</DialogActions>
    </Dialog>
  );
}
BusinessDialog.propTypes = {
  title: PropTypes.string.isRequired,
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  children: PropTypes.node,
  actions: PropTypes.node
};

export function ExportDialog({ value, onClose }) {
  const { t } = useTranslation();
  const content = value.codes ?? value.text;
  const inputRef = useRef(null);
  const [copyStatus, setCopyStatus] = useState('');
  const copyAll = async () => {
    setCopyStatus('');
    try {
      await navigator.clipboard.writeText(content);
      setCopyStatus('success');
    } catch {
      inputRef.current?.focus();
      inputRef.current?.select();
      setCopyStatus('error');
    }
  };
  return (
    <BusinessDialog
      title={t('distribution.exportTitle')}
      open={Boolean(value)}
      onClose={onClose}
      actions={
        <Stack direction="row" spacing={1} useFlexGap sx={{ flexWrap: 'wrap', justifyContent: 'flex-end' }}>
          <Button onClick={onClose}>{t('distribution.close')}</Button>
          <Button
            variant="outlined"
            onClick={() => {
              const blob = new Blob([value.text], { type: 'text/plain;charset=utf-8' });
              const url = URL.createObjectURL(blob);
              const anchor = document.createElement('a');
              anchor.href = url;
              anchor.download = 'distribution.txt';
              anchor.click();
              setTimeout(() => URL.revokeObjectURL(url), 1000);
            }}
          >
            {t('distribution.download')}
          </Button>
          <Button variant="contained" onClick={copyAll} aria-live="polite" aria-atomic="true">
            {t(copyStatus === 'success' ? 'distribution.copied' : value.codes ? 'distribution.copyCards' : 'distribution.copyAll')}
          </Button>
        </Stack>
      }
    >
      <Alert severity="info">{t(value.codes ? 'distribution.cardExportHint' : 'distribution.exportHint')}</Alert>
      <TextField
        inputRef={inputRef}
        label={t(value.codes ? 'distribution.cardCode' : 'distribution.exportContent')}
        value={content}
        multiline
        minRows={6}
        maxRows={16}
        error={copyStatus === 'error'}
        helperText={copyStatus === 'error' ? t('common.copyFailedManual') : undefined}
        slotProps={{ input: { readOnly: true } }}
      />
    </BusinessDialog>
  );
}
ExportDialog.propTypes = {
  value: PropTypes.shape({ text: PropTypes.string.isRequired, codes: PropTypes.string }).isRequired,
  onClose: PropTypes.func.isRequired
};
