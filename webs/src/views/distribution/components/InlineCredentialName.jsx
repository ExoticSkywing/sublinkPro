import { useEffect, useRef, useState } from 'react';
import PropTypes from 'prop-types';
import { useTranslation } from 'react-i18next';
import { Button, Stack, TextField, Typography } from '@mui/material';
import { IconPencil } from '@tabler/icons-react';
import { distributionAPI } from 'api/distribution';
import { errorText } from './Common';

export default function InlineCredentialName({ credential, onSaved }) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(credential.name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const inputRef = useRef(null);
  const buttonRef = useRef(null);
  const restoreFocus = useRef(false);
  const submitting = useRef(false);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    if (!editing && restoreFocus.current) {
      buttonRef.current?.focus();
      restoreFocus.current = false;
    }
  }, [editing]);
  const close = () => {
    restoreFocus.current = true;
    setEditing(false);
    setError('');
  };
  const submit = async (event) => {
    event.preventDefault();
    if (submitting.current) return;
    const value = name.trim();
    if (!value || new TextEncoder().encode(value).length > 100) {
      setError(t(value ? 'distribution.nameTooLong' : 'distribution.nameRequired'));
      inputRef.current?.focus();
      return;
    }
    if (value === credential.name) {
      close();
      return;
    }
    submitting.current = true;
    setBusy(true);
    setError('');
    try {
      // Patch only the name: quick edits must not overwrite rights or resources.
      const result = await distributionAPI.updateCredential(credential.id, { name: value });
      if (!mounted.current) return;
      onSaved(result.data);
      close();
    } catch (err) {
      setError(errorText(err, t));
      inputRef.current?.focus();
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  };
  if (!editing) {
    return credential.status === 'revoked' ? (
      <Typography sx={{ minWidth: 128, maxWidth: 256, overflowWrap: 'anywhere' }}>{credential.name}</Typography>
    ) : (
      <Button
        ref={buttonRef}
        aria-label={t('distribution.renameCredential', { name: credential.name })}
        endIcon={<IconPencil size={16} stroke={1.5} aria-hidden="true" />}
        onClick={() => {
          setName(credential.name);
          setError('');
          setEditing(true);
        }}
        sx={{
          px: 0.5,
          minWidth: 128,
          maxWidth: 256,
          minHeight: { xs: 44, sm: 40 },
          textAlign: 'start',
          justifyContent: 'flex-start',
          textTransform: 'none',
          overflowWrap: 'anywhere',
          '&.Mui-focusVisible': { outline: '2px solid', outlineColor: 'text.primary', outlineOffset: 2 }
        }}
      >
        {credential.name}
      </Button>
    );
  }
  return (
    <Stack
      component="form"
      onSubmit={submit}
      spacing={1}
      sx={{ minWidth: 192, maxWidth: 256 }}
      onKeyDown={(event) => {
        if (event.key === 'Enter' && event.nativeEvent.isComposing) event.preventDefault();
        if (event.key === 'Escape' && !submitting.current) {
          event.preventDefault();
          close();
        }
      }}
    >
      <TextField
        autoFocus
        size="small"
        inputRef={inputRef}
        label={t('distribution.name')}
        value={name}
        onChange={(event) => {
          setName(event.target.value);
          setError('');
        }}
        error={Boolean(error)}
        helperText={error || undefined}
        slotProps={{ htmlInput: { maxLength: 100 }, input: { readOnly: busy } }}
        sx={(theme) => ({
          '& .MuiFormLabel-root.Mui-error, & .MuiFormHelperText-root.Mui-error': {
            color: 'error.dark',
            ...theme.applyStyles('dark', { color: 'error.light' })
          }
        })}
      />
      <Stack direction="row" spacing={1}>
        <Button type="submit" size="small" variant="outlined" loading={busy} sx={{ minHeight: 40 }}>
          {t('distribution.save')}
        </Button>
        <Button size="small" onClick={close} disabled={busy} sx={{ minHeight: 40 }}>
          {t('distribution.cancel')}
        </Button>
      </Stack>
    </Stack>
  );
}
InlineCredentialName.propTypes = { credential: PropTypes.object.isRequired, onSaved: PropTypes.func.isRequired };
