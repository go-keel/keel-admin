import {
  GlobeIcon,
  LanguagesIcon,
  MoonIcon,
  PaletteIcon,
  SunIcon,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useIsDark, useThemePreset } from '@/hooks/use-theme';
import { useTimeZone } from '@/hooks/use-timezone';
import { changeLocale, tDynamic } from '@/lib/i18n';
import {
  getLocale,
  LOCALE_LABELS,
  SUPPORTED_LOCALES,
  type SupportedLocale,
} from '@/lib/locale';
import { setColorMode, setThemePreset } from '@/lib/theme';
import {
  formatTimeZoneOffset,
  listTimeZones,
  setTimeZone,
} from '@/lib/timezone';
import { shell, themePresets } from '@/shell';

/**
 * Shell controls in the header. The tri-state is handled uniformly here: only auto renders; hidden and fixed both
 * render nothing, and they differ in how the value is resolved (see shell.ts). Time zone and language share one
 * dropdown, each axis owning its own section; when no section renders, the button does not either. Appearance preset
 * and color mode are one button each.
 */
function ShellMenu({
  label,
  icon,
  children,
}: {
  label: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="ghost" size="icon" aria-label={label} />}
      >
        {icon}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">{children}</DropdownMenuContent>
    </DropdownMenu>
  );
}

/** One single-choice section inside a dropdown. */
function RadioSection({
  label,
  value,
  onValueChange,
  children,
}: {
  label: string;
  value: string;
  onValueChange: (value: string) => void;
  children: React.ReactNode;
}) {
  return (
    // The label must be inside RadioGroup: Base UI's GroupLabel reads aria-labelledby from the group context and throws
    // outright when placed outside it.
    <DropdownMenuRadioGroup value={value} onValueChange={onValueChange}>
      <DropdownMenuLabel>{label}</DropdownMenuLabel>
      {children}
    </DropdownMenuRadioGroup>
  );
}

function TimeZoneSection() {
  const { t } = useTranslation();
  const current = useTimeZone();
  return (
    <RadioSection
      label={t('timezone.label')}
      value={current}
      onValueChange={setTimeZone}
    >
      {listTimeZones().map((zone) => (
        <DropdownMenuRadioItem key={zone.id} value={zone.id}>
          <span className="mr-3">
            {tDynamic(zone.labelKey, zone.fallbackLabel)}
          </span>
          <span className="ml-auto text-muted-foreground text-xs tabular-nums">
            {formatTimeZoneOffset(zone.id)}
          </span>
        </DropdownMenuRadioItem>
      ))}
    </RadioSection>
  );
}

function LocaleSection() {
  const { t } = useTranslation();
  return (
    <RadioSection
      label={t('locale.label')}
      value={getLocale()}
      onValueChange={(value) => {
        void changeLocale(value as SupportedLocale);
      }}
    >
      {SUPPORTED_LOCALES.map((locale) => (
        <DropdownMenuRadioItem key={locale} value={locale}>
          {LOCALE_LABELS[locale]}
        </DropdownMenuRadioItem>
      ))}
    </RadioSection>
  );
}

/** Header time zone and language: two sections in one dropdown, each axis deciding whether its own section shows. */
export function RegionMenu() {
  const { t } = useTranslation();
  const zone = shell.timeZone.state === 'auto';
  const locale = shell.locale.state === 'auto';
  if (!zone && !locale) return null;
  return (
    <ShellMenu label={t('region.label')} icon={<GlobeIcon />}>
      {zone ? <TimeZoneSection /> : null}
      {zone && locale ? <DropdownMenuSeparator /> : null}
      {locale ? <LocaleSection /> : null}
    </ShellMenu>
  );
}

/** Header appearance preset dropdown. */
export function ThemePresetMenu() {
  const { t } = useTranslation();
  const preset = useThemePreset();
  if (shell.themePreset.state !== 'auto') return null;
  return (
    <ShellMenu label={t('preset.label')} icon={<PaletteIcon />}>
      <RadioSection
        label={t('preset.label')}
        value={preset}
        onValueChange={setThemePreset}
      >
        {themePresets.map((item) => (
          <DropdownMenuRadioItem key={item.id} value={item.id}>
            {item.label}
          </DropdownMenuRadioItem>
        ))}
      </RadioSection>
    </ShellMenu>
  );
}

/**
 * Header color mode toggle: one button that switches to the other mode. The icon shows the current mode; the tooltip
 * says what clicking does.
 *
 * Until chosen it still follows the system (`resolveColorMode` falls back to `system`), and the button shows the mode
 * the system resolves to; the first click fixes it and it no longer follows. There is deliberately no "follow system"
 * option: it would only help when someone already chose manually and wants to go back, at the cost of turning one
 * button into a three-way cycle where you cannot guess what the next click does.
 */
export function ColorModeToggle() {
  const { t } = useTranslation();
  const dark = useIsDark();
  if (shell.colorMode.state !== 'auto') return null;
  const label = t(dark ? 'mode.toLight' : 'mode.toDark');
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            aria-label={label}
            onClick={() => setColorMode(dark ? 'light' : 'dark')}
          />
        }
      >
        {dark ? <MoonIcon /> : <SunIcon />}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/** Language dropdown on the login page: there is no time zone there, so it stands alone. */
export function LocaleMenu() {
  const { t } = useTranslation();
  if (shell.locale.state !== 'auto') return null;
  return (
    <ShellMenu label={t('locale.label')} icon={<LanguagesIcon />}>
      <LocaleSection />
    </ShellMenu>
  );
}
