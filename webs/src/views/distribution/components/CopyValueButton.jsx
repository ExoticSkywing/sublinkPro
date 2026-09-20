import { useEffect, useId, useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Button, ButtonGroup, Menu, MenuItem, TextField } from '@mui/material';
import { IconCheck, IconChevronDown, IconCopy } from '@tabler/icons-react';
import { BusinessDialog } from './Common';

const formats = ['auto', 'clash', 'mihomo', 'surge', 'v2ray'];

// Key by value so rotated links or replacement codes reset the feedback.
export default function CopyValueButton({ value, kind = 'link', showFormats = false }) {
  const { t } = useTranslation();
  const isCard = kind === 'card';
  const hasFormats = showFormats && !isCard;
  const menuId = useId();
  const [anchorEl, setAnchorEl] = useState(null);
  const [status, setStatus] = useState('');
  const [manualValue, setManualValue] = useState('');
  useEffect(() => {
    if (status !== 'copied') return;
    const timer = setTimeout(() => setStatus(''), 2000);
    return () => clearTimeout(timer);
  }, [status]);

  const copy = async (copyValue = value) => {
    if (status === 'copying') return;
    setStatus('copying');
    setManualValue(copyValue);
    try {
      await navigator.clipboard.writeText(copyValue);
      setStatus('copied');
    } catch {
      setStatus('manual');
    }
  };

  return (
    <>
      <ButtonGroup variant="outlined" size="small" sx={{ alignSelf: 'flex-start' }}>
        <Button
          startIcon={status === 'copied' ? <IconCheck size={16} aria-hidden="true" /> : <IconCopy size={16} aria-hidden="true" />}
          disabled={!value}
          aria-disabled={!value || status === 'copying'}
          aria-busy={status === 'copying'}
          onClick={() => copy()}
          aria-live="polite"
          aria-atomic="true"
          sx={{ minWidth: 112, minHeight: { xs: 44, sm: 36 }, whiteSpace: 'nowrap' }}
        >
          {t(status === 'copied' ? 'distribution.copied' : isCard ? 'distribution.copyCard' : 'distribution.copyLinkShort')}
        </Button>
        {hasFormats && (
          <Button
            id={`${menuId}-button`}
            aria-label={t('distribution.copyFormat')}
            aria-haspopup="menu"
            aria-controls={anchorEl ? menuId : undefined}
            aria-expanded={Boolean(anchorEl)}
            disabled={!value || status === 'copying'}
            onClick={(event) => setAnchorEl(event.currentTarget)}
            endIcon={<IconChevronDown size={16} aria-hidden="true" />}
            sx={{ minHeight: { xs: 44, sm: 36 }, whiteSpace: 'nowrap' }}
          >
            {t('distribution.format')}
          </Button>
        )}
      </ButtonGroup>
      {hasFormats && (
        <Menu
          id={menuId}
          anchorEl={anchorEl}
          open={Boolean(anchorEl)}
          onClose={() => setAnchorEl(null)}
          slotProps={{
            list: { 'aria-labelledby': `${menuId}-button` },
            backdrop: { sx: { bgcolor: 'transparent', backdropFilter: 'none' } }
          }}
        >
          {formats.map((format) => (
            <MenuItem
              key={format}
              onClick={() => {
                const url = new URL(value);
                if (format === 'auto') url.searchParams.delete('client');
                else url.searchParams.set('client', format);
                void copy(url.toString());
                setAnchorEl(null);
              }}
              sx={{ minHeight: { xs: 44, sm: 36 } }}
            >
              {t(`subscriptions.share.clients.${format}`)}
            </MenuItem>
          ))}
        </Menu>
      )}
      {status === 'manual' && (
        <BusinessDialog
          title={t(isCard ? 'distribution.copyCard' : 'distribution.copyLink')}
          open
          onClose={() => setStatus('')}
          actions={<Button onClick={() => setStatus('')}>{t('distribution.close')}</Button>}
        >
          <TextField
            label={t(isCard ? 'distribution.cardCode' : 'distribution.subscriptionLink')}
            value={manualValue}
            autoFocus
            multiline
            minRows={3}
            onFocus={(event) => event.target.select()}
            helperText={t('common.copyFailedManual')}
            slotProps={{ input: { readOnly: true } }}
          />
        </BusinessDialog>
      )}
    </>
  );
}

CopyValueButton.propTypes = {
  value: PropTypes.string.isRequired,
  kind: PropTypes.oneOf(['link', 'card']),
  showFormats: PropTypes.bool
};
