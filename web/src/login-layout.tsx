import { useTranslation } from 'react-i18next';
import { LoginForm, LoginHeading } from '@/components/login-form';
import { LocaleMenu } from '@/components/shell-controls';
import { BRAND_LOGO, brandTitle } from '@/lib/brand';

/**
 * Login page layout (assembly seam). The login form and flow are in the kernel's `LoginForm`; this file decides
 * everything around the form: background, columns, the large image on the right, brand, where the language switch sits,
 * the title and description above the form. A derived repo changes it here (add a background effect, switch to a
 * centred card, add a description) without touching the kernel's login page; the upstream baseline is the two-column
 * layout below.
 *
 * Copy uses the app namespace (the derived repo's own `locales/app/`), so changing the title does not touch kernel
 * resources.
 */
export function LoginLayout() {
  const { t } = useTranslation('app');
  return (
    <div className="grid min-h-svh lg:grid-cols-2">
      <div className="flex flex-col gap-4 p-6 md:p-10">
        {/* Brand top left, only the language top right: before login the only thing to switch is whether you can read
            it; color mode follows the system or the last choice. */}
        <div className="flex items-center gap-2 font-medium">
          <img src={BRAND_LOGO} alt="" className="size-6 object-contain" />
          {brandTitle()}
          <div className="ml-auto">
            <LocaleMenu />
          </div>
        </div>
        <div className="flex flex-1 items-center justify-center">
          <div className="w-full max-w-xs">
            <LoginForm
              heading={
                <LoginHeading
                  title={t('login.title')}
                  desc={t('login.subtitle')}
                />
              }
            />
          </div>
        </div>
      </div>
      {/* Large image: replace the single file web/public/login-cover.svg, no code change. On narrow screens the column
          is not rendered, so the image is not downloaded either. Absolutely positioned to fill the right column: in
          normal flow the image stretches the row by its aspect ratio, the page grows taller than the viewport and the
          form is no longer centred. Dimmed and desaturated in dark mode (as in shadcn login-02). */}
      <div className="relative hidden bg-muted lg:block">
        <img
          src="/login-cover.svg"
          alt=""
          className="absolute inset-0 h-full w-full object-cover dark:brightness-[0.2] dark:grayscale"
        />
      </div>
    </div>
  );
}
