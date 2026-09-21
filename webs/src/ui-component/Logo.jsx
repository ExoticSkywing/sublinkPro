import Box from '@mui/material/Box';

import logo from 'assets/images/paraspace-logo-dark.svg';
import useResolvedColorScheme from 'hooks/useResolvedColorScheme';

// ==============================|| LOGO ||============================== //

const Logo = () => {
  const { isDark } = useResolvedColorScheme();

  return (
    <Box
      component="img"
      src={logo}
      alt="ParaSpace"
      sx={{
        display: 'block',
        width: 'auto',
        height: 32,
        filter: isDark ? 'brightness(0) invert(1)' : 'none'
      }}
    />
  );
};

export default Logo;
