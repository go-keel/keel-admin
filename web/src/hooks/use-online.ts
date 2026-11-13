import { useSyncExternalStore } from 'react';

const subscribe = (callback: () => void) => {
  window.addEventListener('online', callback);
  window.addEventListener('offline', callback);
  return () => {
    window.removeEventListener('online', callback);
    window.removeEventListener('offline', callback);
  };
};

const getSnapshot = () =>
  typeof navigator === 'undefined' ? true : navigator.onLine;

export const useOnline = (): boolean =>
  useSyncExternalStore(subscribe, getSnapshot, () => true);
