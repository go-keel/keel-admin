import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { installUpdateWatch } from '@/lib/update';

/** Wiring for update detection: the logic is in lib/update.ts; this only connects it to toast. */
export function UpdateChecker() {
  const { t } = useTranslation();
  useEffect(
    () =>
      installUpdateWatch(() => {
        toast.info(t('update.title'), {
          description: t('update.desc'),
          duration: Number.POSITIVE_INFINITY,
          action: {
            label: t('error.reload'),
            onClick: () => window.location.reload(),
          },
        });
      }),
    [t],
  );
  return null;
}
