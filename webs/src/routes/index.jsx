import { lazy } from 'react';
import { createBrowserRouter, Navigate, useLocation } from 'react-router-dom';
import Loadable from 'ui-component/Loadable';

// routes
import RouterWrapper from './RouterWrapper';
import AuthenticationRoutes from './AuthenticationRoutes';
import MainRoutes from './MainRoutes';
import ErrorBoundary from './ErrorBoundary';

// ==============================|| ROUTING RENDER ||============================== //

// 从后端注入的配置中获取 basePath，回退到环境变量或根路径
const basePath = window.__SUBLINK_CONFIG__?.basePath || import.meta.env.VITE_APP_BASE_NAME || '/';
const DistributionPortal = Loadable(lazy(() => import('views/distribution/Portal')));

// Keep upstream bookmarks and internal links working without moving their source files.
function LegacyAdminRedirect() {
  const location = useLocation();
  return <Navigate to={`/admin${location.pathname}${location.search}${location.hash}`} state={location.state} replace />;
}

const router = createBrowserRouter(
  [
    { path: '/', element: <DistributionPortal />, errorElement: <ErrorBoundary /> },
    ...['login', 'dashboard/*', 'subscription/*', 'script', 'accesskey', 'settings', 'system/*'].map((path) => ({
      path,
      element: <LegacyAdminRedirect />
    })),
    {
      element: <RouterWrapper />,
      errorElement: <ErrorBoundary />,
      children: [MainRoutes, AuthenticationRoutes]
    }
  ],
  {
    basename: basePath
  }
);

export default router;
