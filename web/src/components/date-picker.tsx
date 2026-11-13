import { format, parse } from 'date-fns';
import { CalendarIcon, Clock2Icon, XIcon } from 'lucide-react';
import { type ReactNode, useId, useState } from 'react';
import type { DateRange } from 'react-day-picker';
import { enUS } from 'react-day-picker/locale';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { ButtonGroup } from '@/components/ui/button-group';
import { Calendar } from '@/components/ui/calendar';
import { Field, FieldLabel } from '@/components/ui/field';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Separator } from '@/components/ui/separator';
import { useZone } from '@/hooks/use-timezone';
import { calendarLocaleFor } from '@/languages';
import {
  instantOfWallClock,
  instantRangeOfWall,
  monthRange,
  type RangeValue,
  recentRange,
  todayIn,
  toRFC3339,
  wallClockIn,
  wallRangeOfInstant,
} from '@/lib/time-range';
import { formatTimeZoneShortName } from '@/lib/timezone';
import { cn } from '@/lib/utils';

/*
 * Date and time pickers: a single date, a date range, a single instant, a time range, plus a time of day and a
 * time-of-day range with no date. shadcn gives only an example of combining Popover + Calendar, no ready component;
 * this combines them as in the example and adds three things the example leaves out: which time zone "today" and
 * reading/writing use, the value handed to the form is the value the endpoint wants, and month and weekday names
 * follow the UI language.
 *
 * Values are always strings, passed as in docs/adr/admin-0003-three-meanings-of-time-and-how-to-pass-them.md: a date
 * is `YYYY-MM-DD`, an instant is an RFC3339 UTC string (to the second); a range is `{ from, to }`, either end may be
 * `''`. A date range includes both ends; a time range is half-open: the UI includes both ends to the second, and the
 * end is pushed one second later when handed over, so forms and pages do no conversion.
 */

const DAY = 'yyyy-MM-dd';
const DAY_START = '00:00:00';
const DAY_END = '23:59:59';
const EMPTY: RangeValue = { from: '', to: '' };

const toDate = (s: string) => (s ? parse(s, DAY, new Date()) : undefined);

/**
 * Which time zone a date is read in: `display` is the display time zone in the top bar, `business` is the business
 * time zone; without it the value is a calendar date (birthdays, contract dates and the like have no time zone),
 * "today" is marked in the display time zone and the trigger shows no time zone.
 */
export type DateZone = 'display' | 'business';

/** Props shared by the four popover pickers. */
type PickerProps = {
  id?: string;
  /** Added to the trigger button, such as a fixed width in a form. */
  className?: string;
  disabled?: boolean;
  /**
   * A label written into the trigger button before the value (the filter bar's "Range:"); form field labels still use
   * FieldLabel.
   */
  label?: ReactNode;
  /** Trigger text when nothing is selected; by default a grey "Select date" / "Select time". */
  placeholder?: ReactNode;
  'aria-invalid'?: boolean;
  'aria-required'?: boolean;
};

/**
 * The effective time zone marked on the trigger button: whose day "September 1" is must be written on the control.
 */
function ZoneMark({ zone }: { zone: string }) {
  return (
    <span className="text-muted-foreground text-xs">
      ({formatTimeZoneShortName(zone)})
    </span>
  );
}

/**
 * The trigger button's background and border follow the triggers of inputs and selects (transparent background,
 * `border-input`): an outline button's background is the page background color, and when an appearance preset's page
 * background is not pure white, inside a white dialog or card it is a shade darker than the inputs next to it and
 * looks disabled. Dark mode and the open state keep the outline button's own background.
 */
const TRIGGER = 'border-input bg-transparent';

/**
 * Popover + trigger button + clear button. The clear button sits next to the trigger button in a group, not inside
 * it: a button inside a button is not valid HTML.
 */
function PickerShell({
  props: {
    id,
    className,
    disabled,
    label,
    placeholder,
    'aria-invalid': invalid,
    'aria-required': required,
  },
  text,
  zone,
  time,
  onClear,
  children,
}: {
  props: PickerProps;
  /** The value on the trigger button; `''` means nothing selected. */
  text: string;
  /** The time zone to mark (an IANA name); none when omitted. */
  zone?: string;
  /** Whether the default placeholder says "Select time" or "Select date". */
  time?: boolean;
  onClear: () => void;
  children: (close: () => void) => ReactNode;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <ButtonGroup>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          id={id}
          disabled={disabled}
          aria-invalid={invalid}
          aria-required={required}
          render={
            <Button
              variant="outline"
              className={cn(TRIGGER, 'justify-start font-normal', className)}
            />
          }
        >
          <CalendarIcon data-icon="inline-start" />
          {label}
          <span className="truncate">
            {text ||
              (placeholder ?? (
                <span className="text-muted-foreground">
                  {t(time ? 'datePicker.pickTime' : 'datePicker.pickDate')}
                </span>
              ))}
          </span>
          {zone ? <ZoneMark zone={zone} /> : null}
        </PopoverTrigger>
        <PopoverContent className="w-auto flex-row gap-0 p-0" align="start">
          {children(() => setOpen(false))}
        </PopoverContent>
      </Popover>
      {text && !disabled ? (
        <Button
          type="button"
          variant="outline"
          size="icon"
          className={TRIGGER}
          aria-label={t('datePicker.clear')}
          onClick={onClear}
        >
          <XIcon />
        </Button>
      ) : null}
    </ButtonGroup>
  );
}

const HIDE_NATIVE_PICKER =
  'appearance-none [&::-webkit-calendar-picker-indicator]:hidden [&::-webkit-calendar-picker-indicator]:appearance-none';

/**
 * Time input value → `HH:mm` or `HH:mm:ss`: browsers may give only `HH:mm` when seconds are 0, so pad when seconds are
 * wanted.
 */
const normalizeTime = (v: string, seconds?: boolean): string =>
  !v ? '' : seconds && v.length === 5 ? `${v}:00` : v.slice(0, seconds ? 8 : 5);

/**
 * Picks a time of day with no date (such as "send at what time every day"). As in shadcn's Date and Time Picker
 * example: type hours and minutes into a time input, hide the browser's own dropdown button (it looks different in
 * every browser and ignores the theme) and show a clock icon instead.
 *
 * The value is `HH:mm`, or `HH:mm:ss` with `seconds`, and `''` when empty; browsers may give only `HH:mm` when seconds
 * are 0, padded here. `zone` only marks at the end of the input which time zone the value is read in; the value itself
 * is not converted. `className` goes on the input's outer box; set a fixed width directly (such as `w-36`).
 */
export function TimePicker({
  id,
  className,
  disabled,
  value,
  onChange,
  seconds,
  zone,
  'aria-invalid': invalid,
  'aria-required': required,
}: {
  id?: string;
  className?: string;
  disabled?: boolean;
  value?: string;
  onChange: (value: string) => void;
  seconds?: boolean;
  zone?: DateZone;
  'aria-invalid'?: boolean;
  'aria-required'?: boolean;
}) {
  const z = useDateZone(zone);
  return (
    // A vertical Field stretches every direct child to the full row; a fixed width in className works only behind this
    // layer.
    <div>
      <InputGroup className={className}>
        <InputGroupInput
          id={id}
          type="time"
          step={seconds ? 1 : undefined}
          value={value ?? ''}
          disabled={disabled}
          aria-invalid={invalid}
          aria-required={required}
          className={HIDE_NATIVE_PICKER}
          onChange={(e) => onChange(normalizeTime(e.target.value, seconds))}
        />
        <InputGroupAddon>
          <Clock2Icon className="text-muted-foreground" />
        </InputGroupAddon>
        {z.mark ? (
          <InputGroupAddon align="inline-end">
            <ZoneMark zone={z.mark} />
          </InputGroupAddon>
        ) : null}
      </InputGroup>
    </div>
  );
}

/**
 * Picks a range of times within a day, with no date (such as "send every day from 09:00 to 21:00"). The value is
 * `{ from, to }`, each end as in `TimePicker`, either end may be `''`; an end earlier than the start (across midnight)
 * is for the business logic to interpret, not blocked or converted here. `className` goes on the row holding both
 * inputs, `id` goes to the start input (the target of `FieldLabel`'s `htmlFor`).
 */
export function TimeRangePicker({
  id,
  className,
  disabled,
  value,
  onChange,
  seconds,
  zone,
  'aria-invalid': invalid,
  'aria-required': required,
}: {
  id?: string;
  className?: string;
  disabled?: boolean;
  value?: RangeValue;
  onChange: (value: RangeValue) => void;
  seconds?: boolean;
  zone?: DateZone;
  'aria-invalid'?: boolean;
  'aria-required'?: boolean;
}) {
  const { t } = useTranslation();
  const z = useDateZone(zone);
  const { from, to } = value ?? EMPTY;
  const end = (v: string, side: 'from' | 'to', label: string) => (
    <InputGroup>
      <InputGroupInput
        id={side === 'from' ? id : undefined}
        aria-label={side === 'to' ? label : undefined}
        type="time"
        step={seconds ? 1 : undefined}
        value={v}
        disabled={disabled}
        aria-invalid={invalid}
        aria-required={required}
        className={HIDE_NATIVE_PICKER}
        onChange={(e) =>
          onChange({
            from,
            to,
            [side]: normalizeTime(e.target.value, seconds),
          })
        }
      />
      <InputGroupAddon>
        <Clock2Icon className="text-muted-foreground" />
      </InputGroupAddon>
    </InputGroup>
  );
  return (
    // A vertical Field stretches every direct child to the full row; a fixed width in className works only behind this
    // layer.
    <div>
      <div className={cn('flex items-center gap-2', className)}>
        {end(from, 'from', t('datePicker.fromTime'))}
        <span className="text-muted-foreground">~</span>
        {end(to, 'to', t('datePicker.toTime'))}
        {z.mark ? <ZoneMark zone={z.mark} /> : null}
      </div>
    </div>
  );
}

/**
 * The hour-minute-second input inside a date-time picker's popover: labelled; clearing returns to `fallback` (that
 * end's default time).
 */
function TimeField({
  label,
  value,
  fallback,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  fallback: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <TimePicker
        id={id}
        seconds
        value={value}
        disabled={disabled}
        onChange={(v) => onChange(v || fallback)}
      />
    </Field>
  );
}

/** Quick ranges, computing start and end dates from today in the given time zone. "Last N days" includes today. */
const PRESETS = [
  { key: 'today', range: (zone: string) => recentRange(0, 0, zone) },
  { key: 'yesterday', range: (zone: string) => recentRange(1, 1, zone) },
  { key: 'last3', range: (zone: string) => recentRange(2, 0, zone) },
  { key: 'last7', range: (zone: string) => recentRange(6, 0, zone) },
  { key: 'last30', range: (zone: string) => recentRange(29, 0, zone) },
  { key: 'thisMonth', range: (zone: string) => monthRange(0, zone) },
  { key: 'lastMonth', range: (zone: string) => monthRange(-1, zone) },
] as const;

/**
 * The column of quick ranges to the right of the calendar, shared by the two range pickers; clicking one selects it and
 * closes the popover.
 */
function PresetList({
  zone,
  onPick,
}: {
  zone: string;
  onPick: (from: string, to: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Separator orientation="vertical" />
      <div className="flex flex-col gap-1 p-3">
        {PRESETS.map((p) => (
          <Button
            key={p.key}
            type="button"
            variant="ghost"
            size="sm"
            className="justify-start"
            onClick={() => {
              const [from = '', to = ''] = p.range(zone).split('~');
              onPick(from, to);
            }}
          >
            {t(`datePicker.preset.${p.key}`)}
          </Button>
        ))}
      </div>
    </>
  );
}

/**
 * Shared calendar props: month and weekday names follow the UI language (a Chinese week starts on Monday), "today" is
 * taken in the given time zone.
 */
function useCalendarProps(zone: string) {
  const { i18n } = useTranslation();
  return {
    today: toDate(todayIn(zone)),
    locale: calendarLocaleFor(i18n.language) ?? enUS,
  };
}

/**
 * `DateZone` → time zone name; for a calendar date (omitted) "today" is taken in the display time zone but not marked.
 */
function useDateZone(zone: DateZone | undefined) {
  const name = useZone(zone);
  return { name, mark: zone ? name : undefined };
}

/** Picks one date. The value is `YYYY-MM-DD`, `''` when nothing is selected. */
export function DatePicker({
  value,
  onChange,
  zone,
  ...props
}: PickerProps & {
  value?: string;
  onChange: (value: string) => void;
  zone?: DateZone;
}) {
  const z = useDateZone(zone);
  const calendar = useCalendarProps(z.name);
  const selected = toDate(value ?? '');
  return (
    <PickerShell
      props={props}
      text={value ?? ''}
      zone={z.mark}
      onClear={() => onChange('')}
    >
      {() => (
        <Calendar
          mode="single"
          selected={selected}
          defaultMonth={selected}
          {...calendar}
          onSelect={(d) => onChange(d ? format(d, DAY) : '')}
        />
      )}
    </PickerShell>
  );
}

/**
 * Picks a range of dates, start and end in one go, both ends included. The calendar holds plain dates without times,
 * so the browser's time zone does not affect which day is selected. `presets` adds a column of quick ranges on the
 * right (today, last 7 days...).
 */
export function DateRangePicker({
  value,
  onChange,
  zone,
  presets,
  ...props
}: PickerProps & {
  value?: RangeValue;
  onChange: (value: RangeValue) => void;
  zone?: DateZone;
  presets?: boolean;
}) {
  const z = useDateZone(zone);
  const calendar = useCalendarProps(z.name);
  const { from, to } = value ?? EMPTY;
  const selected: DateRange | undefined =
    from || to ? { from: toDate(from), to: toDate(to) } : undefined;
  return (
    <PickerShell
      props={props}
      text={from || to ? `${from || '…'} ~ ${to || '…'}` : ''}
      zone={z.mark}
      onClear={() => onChange(EMPTY)}
    >
      {(close) => (
        <>
          <Calendar
            mode="range"
            numberOfMonths={2}
            selected={selected}
            defaultMonth={selected?.from}
            {...calendar}
            onSelect={(r) =>
              onChange({
                from: r?.from ? format(r.from, DAY) : '',
                to: r?.to ? format(r.to, DAY) : '',
              })
            }
          />
          {presets ? (
            <PresetList
              zone={z.name}
              onPick={(f, t) => {
                onChange({ from: f, to: t });
                close();
              }}
            />
          ) : null}
        </>
      )}
    </PickerShell>
  );
}

/**
 * Picks one instant (to the second): the calendar picks the date and the time input below sets it to the second,
 * read, written and marked in `zone` (the top bar's display time zone when omitted; `business` for instants that may
 * only be entered in the business time zone). The value is an RFC3339 UTC string, `''` when nothing is selected.
 *
 * `defaultAt`: which time the first click on a date lands on when nothing has been selected yet (the argument is the
 * selected date `YYYY-MM-DD`, read in the same time zone; returns an RFC3339 UTC string), such as an expiry defaulting
 * to the end of that day; 00:00:00 of that day when omitted. Changing the date afterwards keeps the selected time.
 */
export function DateTimePicker({
  value,
  onChange,
  defaultAt,
  zone: zoneOf = 'display',
  ...props
}: PickerProps & {
  value?: string;
  onChange: (value: string) => void;
  defaultAt?: (day: string) => string;
  zone?: DateZone;
}) {
  const { t } = useTranslation();
  const zone = useDateZone(zoneOf).name;
  const calendar = useCalendarProps(zone);
  const wall = value ? wallClockIn(new Date(value), zone) : '';
  const [day = '', time = DAY_START] = wall ? wall.split(' ') : [];
  const emit = (d: string, tm: string) =>
    onChange(d ? toRFC3339(instantOfWallClock(`${d} ${tm}`, zone)) : '');
  const selected = toDate(day);
  return (
    <PickerShell
      props={props}
      text={wall}
      zone={zone}
      time
      onClear={() => onChange('')}
    >
      {() => (
        <div className="flex flex-col">
          <Calendar
            mode="single"
            selected={selected}
            defaultMonth={selected}
            {...calendar}
            onSelect={(d) => {
              const picked = d ? format(d, DAY) : '';
              if (picked && !day && defaultAt) onChange(defaultAt(picked));
              else emit(picked, time);
            }}
          />
          <Separator />
          <div className="p-3">
            <TimeField
              label={t('datePicker.time')}
              value={time}
              fallback={DAY_START}
              disabled={!day}
              onChange={(tm) => emit(day, tm)}
            />
          </div>
        </div>
      )}
    </PickerShell>
  );
}

/**
 * Picks a time range (to the second, read, written and marked in `zone`, the top bar's display time zone when
 * omitted): the calendar picks the start and end days, and the two time inputs below set each to the second. When
 * only dates are picked (including a quick range) the start is 00:00:00 and the end 23:59:59, the whole days. The value
 * is a range of UTC instants `{ from, to }`, half-open: an end of 23:59:59 in the UI is handed over as 00:00:00 the
 * next day.
 */
export function DateTimeRangePicker({
  value,
  onChange,
  presets,
  zone: zoneOf = 'display',
  ...props
}: PickerProps & {
  value?: RangeValue;
  onChange: (value: RangeValue) => void;
  presets?: boolean;
  zone?: DateZone;
}) {
  const { t } = useTranslation();
  const zone = useDateZone(zoneOf).name;
  const calendar = useCalendarProps(zone);
  const wall = wallRangeOfInstant(value ?? EMPTY, zone);
  const [fromDay = '', fromTime = DAY_START] = wall.from.split(' ');
  const [toDay = '', toTime = DAY_END] = wall.to.split(' ');
  const emit = (fd: string, ft: string, td: string, tt: string) =>
    onChange(
      instantRangeOfWall(
        { from: fd ? `${fd} ${ft}` : '', to: td ? `${td} ${tt}` : '' },
        zone,
      ),
    );
  const selected: DateRange | undefined =
    fromDay || toDay ? { from: toDate(fromDay), to: toDate(toDay) } : undefined;
  return (
    <PickerShell
      props={props}
      text={
        wall.from || wall.to ? `${wall.from || '…'} ~ ${wall.to || '…'}` : ''
      }
      zone={zone}
      time
      onClear={() => onChange(EMPTY)}
    >
      {(close) => (
        <>
          <div className="flex flex-col">
            <Calendar
              mode="range"
              numberOfMonths={2}
              selected={selected}
              defaultMonth={selected?.from}
              {...calendar}
              onSelect={(r) =>
                emit(
                  r?.from ? format(r.from, DAY) : '',
                  fromTime,
                  r?.to ? format(r.to, DAY) : '',
                  toTime,
                )
              }
            />
            <Separator />
            <div className="flex gap-3 p-3">
              <TimeField
                label={t('datePicker.fromTime')}
                value={fromTime}
                fallback={DAY_START}
                disabled={!fromDay}
                onChange={(tm) => emit(fromDay, tm, toDay, toTime)}
              />
              <TimeField
                label={t('datePicker.toTime')}
                value={toTime}
                fallback={DAY_END}
                disabled={!toDay}
                onChange={(tm) => emit(fromDay, fromTime, toDay, tm)}
              />
            </div>
          </div>
          {presets ? (
            <PresetList
              zone={zone}
              onPick={(f, tt) => {
                emit(f, DAY_START, tt, DAY_END);
                close();
              }}
            />
          ) : null}
        </>
      )}
    </PickerShell>
  );
}
