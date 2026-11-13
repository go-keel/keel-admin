import { WifiOffIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useOnline } from '@/hooks/use-online';

/** Offline banner. If the browser says it is offline, it is; no probe request of our own to double-check. */
export function OfflineBanner() {
  const { t } = useTranslation();
  const online = useOnline();
  if (online) return null;
  return (
    <div
      role="status"
      className="-translate-x-1/2 fixed top-2 left-1/2 z-50 flex items-center gap-2 rounded-md border border-border bg-popover px-3 py-2 text-popover-foreground text-sm shadow-lg"
    >
      <WifiOffIcon className="size-4" />
      {t('network.offline')}
    </div>
  );
}
