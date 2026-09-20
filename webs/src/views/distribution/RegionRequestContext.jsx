import PropTypes from 'prop-types';
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { distributionAPI } from 'api/distribution';

const RegionRequestContext = createContext(null);

// Mounted only inside AuthGuard. Share one count across the header and page;
// keep pending work separate from dismissible notification history.
export function RegionRequestProvider({ children }) {
  const [count, setCount] = useState(null);
  const [error, setError] = useState(false);
  const active = useRef(false);
  const request = useRef(null);
  const refresh = useCallback(async () => {
    if (!active.current) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    try {
      const result = await distributionAPI.pendingRegions(controller.signal);
      if (controller.signal.aborted || !active.current) return;
      if (!Number.isInteger(result.data?.total) || result.data.total < 0) throw new Error('Invalid count');
      setCount(result.data.total);
      setError(false);
    } catch {
      if (!controller.signal.aborted && active.current) setError(true);
    }
  }, []);

  useEffect(() => {
    active.current = true;
    const whenVisible = () => {
      if (document.visibilityState === 'visible') refresh();
    };
    refresh();
    const timer = setInterval(whenVisible, 30000);
    window.addEventListener('focus', whenVisible);
    document.addEventListener('visibilitychange', whenVisible);
    return () => {
      active.current = false;
      request.current?.abort();
      clearInterval(timer);
      window.removeEventListener('focus', whenVisible);
      document.removeEventListener('visibilitychange', whenVisible);
    };
  }, [refresh]);

  const value = useMemo(() => ({ count, error, refresh }), [count, error, refresh]);
  return <RegionRequestContext.Provider value={value}>{children}</RegionRequestContext.Provider>;
}

RegionRequestProvider.propTypes = { children: PropTypes.node };

export const useRegionRequests = () => useContext(RegionRequestContext);
