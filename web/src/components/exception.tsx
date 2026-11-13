import { Link, useRouter } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';

/** Layout shared by the three exception pages: a large status, one sentence of explanation, one or two ways out. */
function ExceptionPanel({
  status,
  desc,
  children,
}: {
  status: string;
  desc: string;
  children?: React.ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 p-6 text-center">
      <p className="font-semibold text-6xl text-muted-foreground tabular-nums">
        {status}
      </p>
      <p className="max-w-prose text-muted-foreground text-sm">{desc}</p>
      <div className="flex gap-2">
        {children}
        {/* When rendered as <a>, nativeButton must be turned off explicitly, or Base UI warns that "button semantics
            were lost": by default it assumes render still produces a <button>. */}
        <Button nativeButton={false} render={<Link to="/" />}>
          {t('error.home')}
        </Button>
      </div>
    </div>
  );
}

export function Forbidden() {
  const { t } = useTranslation();
  return (
    <ExceptionPanel
      status={t('error.forbidden.title')}
      desc={t('error.forbidden.desc')}
    />
  );
}

export function NotFound() {
  const { t } = useTranslation();
  return (
    <ExceptionPanel
      status={t('error.notFound.title')}
      desc={t('error.notFound.desc')}
    />
  );
}

/**
 * The route's errorComponent. Failed requests do not land here: a failed fetch affects only the block that uses that
 * data (docs/admin/errors.md, "A failed fetch affects only its block"). Only two things land here: the frontend code
 * itself failing to render, and the full-screen failure when the auth shell cannot get the profile (server error,
 * offline).
 *
 * Retry must rerun this level's fetch (`router.invalidate()`) and then clear the error state: clearing only the state
 * leaves a shell-type error in the route match, and redrawing shows this panel again.
 */
export function ServerError({ reset }: { reset?: () => void }) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <ExceptionPanel
      status={t('error.server.title')}
      desc={t('error.server.desc')}
    >
      <Button
        variant="outline"
        onClick={() => {
          void router.invalidate();
          reset?.();
        }}
      >
        {t('error.retry')}
      </Button>
    </ExceptionPanel>
  );
}
