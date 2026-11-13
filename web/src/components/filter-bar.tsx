import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { type ReactNode, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { LoadFailed } from '@/components/data-region';
import { DateRangePicker, DateTimeRangePicker } from '@/components/date-picker';
import { Button } from '@/components/ui/button';
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxTrigger,
  ComboboxValue,
} from '@/components/ui/combobox';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useTimeZone } from '@/hooks/use-timezone';
import { loadState, type QueryLike, retryFailed } from '@/lib/load-state';
import { nextBatchOffset } from '@/lib/table';
import {
  formatRange,
  instantRangeOfWall,
  parseRange,
  wallRangeOfInstant,
} from '@/lib/time-range';

/**
 * The filter bar of a list page. Declarative: given a list of fields, it renders the controls, keeps a draft, and on
 * "Search" hands the whole draft over at once.
 *
 * It has no shell of its own: pass it as `DataTable`'s `toolbar`, and the header section of the table card wraps it.
 *
 * **Explicitly triggered**, not searching as you type: a list request should not run on every keystroke, and the URL
 * would fill with intermediate states that the browser's Back button cannot get out of.
 *
 * Values are always strings: they end up in the URL search, so they take their final form here, saving a round of
 * conversion (a numeric filter constrains input with an input of type `number`; parsing is in the route's zod schema).
 */
export type FilterOption = { value: string; label: string };

/**
 * The data source of a searchable select, provided by the page (the kernel handles only the interaction). `load` maps
 * one-to-one to the parameters of a batched options endpoint: with `ids` it returns only those items (to show the
 * names of selected items), otherwise it fetches by keyword in batches, returning the first batch for an empty keyword.
 * Usually exported once from the domain's API file and spread into several pages (such as `userOptionsSource` in
 * `lib/users.ts`).
 */
export type SearchSelectSource = {
  /** Cache prefix: the same data source uses the same one, different sources must not collide. */
  queryKey: readonly unknown[];
  /** Items per batch, at most the endpoint's limit. */
  pageSize: number;
  load: (query: {
    keyword?: string;
    offset?: number;
    limit?: number;
    ids?: string[];
  }) => Promise<{ items: FilterOption[]; total: number }>;
};

/**
 * One field of the filter bar. `K` is the range of keys: used through `useList` it takes the list definition's keys,
 * so a misspelled key (not in the list definition) does not compile; otherwise the item would still render, but the
 * selected value would be dropped by the list definition on submit and the filter would silently not apply. Used on
 * its own (without `useList`) it is any string.
 */
export type FilterField<K extends string = string> =
  // Without a placeholder, the label is used: inputs have no label, so the label serves as the placeholder.
  | { key: K; label: string; type: 'input'; placeholder?: string }
  | { key: K; label: string; type: 'number'; placeholder?: string }
  | {
      key: K;
      label: string;
      type: 'select';
      options: FilterOption[];
      /**
       * When the options come from an endpoint, pass that query's result: if it failed, the dropdown shows "Failed to
       * load" and a retry.
       */
      load?: QueryLike;
      /**
       * Turns off the automatic "All" item, for selects that cannot have "All" (a concrete value must be selected).
       * Off by default: most filter selects should have "All"; a derived repo cannot change the kernel, so the switch
       * is provided up front.
       */
      hideAllOption?: boolean;
    }
  | {
      key: K;
      label: string;
      type: 'treeSelect';
      /** Options already indented by level (the page flattens the tree; the filter bar knows nothing of trees). */
      options: FilterOption[];
      load?: QueryLike;
      hideAllOption?: boolean;
    }
  /**
   * Time filters, matching one-to-one how the same-named field is written in the list definition (`ListField` in
   * `lib/list.ts`): `zone: 'business'` is the fixed kind and picks business days in the business time zone; without it
   * is the following kind and picks in the display time zone.
   */
  | { key: K; label: string; type: 'dateRange'; zone?: 'business' }
  | { key: K; label: string; type: 'timeRange' }
  /**
   * Searchable select: for options that keep growing with the business and cannot be given all at once (see
   * GLOSSARY.md, "option set").
   */
  | ({ key: K; label: string; type: 'searchSelect' } & SearchSelectSource)
  /**
   * A page-defined control: the filter bar hands `render` the value being edited for this field and its setter;
   * search, reset and backfill work as for built-in fields. The value is still a string (it ends up in the URL). Write
   * the label into the control the way the built-in select does; `label` is the "Label:" prefix the filter bar has
   * prepared, put it straight into the control.
   */
  | {
      key: K;
      label: string;
      type: 'custom';
      render: (props: {
        value: string;
        onChange: (value: string) => void;
        label: ReactNode;
      }) => ReactNode;
    };

export type FilterValues<K extends string = string> = Record<K, string>;

/*
 * Laid out in one row, without a separate label row (as in the reference shadcn admin): selects and date ranges write
 * the label inside the control ("Status: All"), inputs have no label and use it as the placeholder; an input's value is
 * what the user typed, so it needs no explanation like "All", and "Keyword" in the box says what to enter. This saves a
 * label row's height, and "Search" is still explicitly triggered.
 *
 * Filter controls always use the component's default size and set no height: inputs, selects and buttons of the
 * default size are already the same height; mixing in an sm one or a hand-written height misaligns them, and every
 * component size change would need following.
 */

export function FilterBar<K extends string = string>({
  fields,
  values,
  onSubmit,
  onReset,
  actions,
  revision = 0,
}: {
  fields: FilterField<K>[];
  /** The current values from the URL: refresh, shared links, Back and Forward all backfill from here. */
  values: FilterValues<K>;
  /**
   * Incremented to reset the draft to `values` even when the values have not changed (such as clicking a sidebar link
   * to this page); given by `useList`.
   */
  revision?: number;
  onSubmit: (values: FilterValues<K>) => void;
  onReset: () => void;
  /**
   * Buttons next to "Search" and "Reset". **Export goes here**: it acts on the current set of filters, so it is part
   * of the filter module, not a filter-independent feature in the page header (like Create).
   */
  actions?: ReactNode;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<FilterValues<K>>(values);

  // When the URL changes (Back, arriving with filter values from outside), backfill the draft again, but compare by
  // value, not by object reference. The page computes a new values object on every render; with a useEffect depending
  // on it, any re-render (even one triggered by a background refetch on window focus) would rerun the backfill and wipe
  // the draft the user just typed and has not submitted. This uses the "adjusting state during render" pattern from the
  // React docs (compare in the render body, setState when needed) rather than useEffect plus a serialised dependency:
  // when the value has not changed nothing happens, and there is no fight with the exhaustive-deps check.
  const valuesKey = JSON.stringify([revision, values]);
  const [prevValuesKey, setPrevValuesKey] = useState(valuesKey);
  if (valuesKey !== prevValuesKey) {
    setPrevValuesKey(valuesKey);
    setDraft(values);
  }

  const set = (key: K, value: string) =>
    setDraft((prev) => ({ ...prev, [key]: value }));

  return (
    <form
      // No border or padding: it lives in the header section of the DataTable card, and the table provides the shell.
      className="flex flex-wrap items-center gap-3"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit(draft);
      }}
    >
      {fields.map((field) => (
        <FilterControl
          key={field.key}
          field={field}
          value={draft[field.key] ?? ''}
          onChange={(v) => set(field.key, v)}
        />
      ))}
      <div className="flex items-center gap-2">
        <Button type="submit">{t('filter.search')}</Button>
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            setDraft({} as FilterValues<K>);
            onReset();
          }}
        >
          {t('filter.reset')}
        </Button>
        {actions}
      </div>
    </form>
  );
}

function FilterControl({
  field,
  value,
  onChange,
}: {
  field: FilterField;
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const displayZone = useTimeZone();
  // The label prefix in selects and date ranges, in grey to set it apart from the value after it.
  const label = (
    <span className="text-muted-foreground">
      {t('format.labeled', { label: field.label })}
    </span>
  );
  if (field.type === 'select' || field.type === 'treeSelect') {
    // "All" = no value, added automatically as the first item (value ''): so no filter select writes it by hand, and
    // the
    // value is uniformly '': "no value" is the same value everywhere. The field's hideAllOption turns it off, for
    // selects that cannot have "All".
    const options = field.hideAllOption
      ? field.options
      : [{ value: '', label: t('filter.all') }, ...field.options];
    // An empty value falls back to the first option: without this key in the URL the select should not be blank (its
    // width follows the content, and with no text only an arrow is left). With "All" the first item is already that, so
    // this only covers hideAllOption with a current value that is not among the options.
    const current = options.some((o) => o.value === value)
      ? value
      : (options[0]?.value ?? '');
    return (
      <Select
        value={current}
        onValueChange={(v) => onChange(v ?? '')}
        // Let Select map value to text: without items the trigger shows only the raw value ("0").
        items={Object.fromEntries(options.map((o) => [o.value, o.label]))}
      >
        {/* The label is text inside the trigger, so the name a screen reader reads is "Status: All"; no aria-label
            needed. */}
        <SelectTrigger>
          {label}
          <SelectValue />
        </SelectTrigger>
        {/* Opens from the trigger's bottom-left corner. The default "selected item over the trigger text" aligns
            the list with the value text, and since a label precedes the value, the whole list shifts right and
            covers the trigger. */}
        <SelectContent align="start" alignItemWithTrigger={false}>
          <SelectGroup>
            {options.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectGroup>
          {/* Options failed to load: not treated as "nothing to choose"; say so below the list and offer a retry. */}
          {field.load && loadState([field.load]) === 'failed' ? (
            <LoadFailed
              className="py-3"
              onRetry={() => retryFailed(field.load ? [field.load] : [])}
            />
          ) : null}
        </SelectContent>
      </Select>
    );
  }
  // A time filter's value stays in the URL as a human-readable `start~end`; the control's value is what the endpoint
  // takes, converted here. A time range is stored in the URL as display-time-zone wall time, and after the display time
  // zone changes it is read in the new zone (consistent with the conversion when fetching).
  if (field.type === 'dateRange') {
    return (
      <DateRangePicker
        label={label}
        placeholder={t('filter.all')}
        zone={field.zone ?? 'display'}
        presets
        value={parseRange(value, 'date')}
        onChange={(r) => onChange(formatRange(r))}
      />
    );
  }
  if (field.type === 'timeRange') {
    const range = parseRange(value, 'time');
    return (
      <DateTimeRangePicker
        label={label}
        placeholder={t('filter.all')}
        presets
        value={range && instantRangeOfWall(range, displayZone)}
        onChange={(r) =>
          onChange(formatRange(wallRangeOfInstant(r, displayZone)))
        }
      />
    );
  }
  if (field.type === 'searchSelect') {
    return (
      <SearchSelectControl
        label={label}
        source={field}
        value={value}
        onChange={onChange}
      />
    );
  }
  if (field.type === 'custom') {
    return field.render({ value, onChange, label });
  }
  return (
    <Input
      // A placeholder disappears once you type, so the name must be given to the screen reader separately. Input fills
      // its parent by default (w-full); in a row it must shrink to its content.
      aria-label={field.label}
      className="w-auto"
      type={field.type === 'number' ? 'number' : 'text'}
      inputMode={field.type === 'number' ? 'numeric' : undefined}
      placeholder={field.placeholder ?? field.label}
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}

/** Fetch by keyword only after typing pauses this long, not a request on every keystroke. */
const SEARCH_DEBOUNCE_MS = 300;

function useDebounced<T>(value: T, ms: number): T {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setSettled(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return settled;
}

/**
 * Searchable select (shadcn Combobox with the component's own client-side filtering off; all options come from the
 * server through `load`): once open it fetches in batches by the typed keyword, the first batch for an empty keyword,
 * and the next batch when scrolled to the bottom; the selected item's name is fetched separately with `ids`, so it
 * shows after a refresh or on a shared link too. With an empty keyword the first item is "All", and choosing it clears
 * the value, the same wording as an ordinary select.
 */
function SearchSelectControl({
  label,
  source,
  value,
  onChange,
}: {
  label: ReactNode;
  source: SearchSelectSource;
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [keyword, setKeyword] = useState('');
  const settledKeyword = useDebounced(keyword.trim(), SEARCH_DEBOUNCE_MS);

  const batches = useInfiniteQuery({
    queryKey: [...source.queryKey, 'search', settledKeyword, source.pageSize],
    queryFn: ({ pageParam }) =>
      source.load({
        keyword: settledKeyword,
        offset: pageParam,
        limit: source.pageSize,
      }),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => nextBatchOffset(pages, last.total),
    enabled: open,
  });
  const picked = useQuery({
    queryKey: [...source.queryKey, 'ids', value],
    queryFn: () => source.load({ ids: [value] }),
    enabled: value !== '',
  });

  // A batch arrived but the list does not fill a screen yet (no scrollbar, so no scrolling to the bottom), and more
  // remain: fetch on.
  const listRef = useRef<HTMLDivElement>(null);
  const { data, hasNextPage, isFetchingNextPage, fetchNextPage } = batches;
  useEffect(() => {
    const el = listRef.current;
    if (!el || !data || !hasNextPage || isFetchingNextPage) return;
    if (el.scrollHeight <= el.clientHeight) void fetchNextPage();
  }, [data, hasNextPage, isFetchingNextPage, fetchNextPage]);

  const all: FilterOption = { value: '', label: t('filter.all') };
  const loaded = batches.data?.pages.flatMap((p) => p.items) ?? [];
  const items = settledKeyword ? loaded : [all, ...loaded];
  // When the name cannot be fetched (record deleted, no permission), show the raw value, not a blank.
  const selected: FilterOption = value
    ? (picked.data?.items[0] ??
      loaded.find((o) => o.value === value) ?? { value, label: value })
    : all;

  return (
    <Combobox
      items={items}
      filter={null}
      value={selected}
      onValueChange={(o: FilterOption | null) => onChange(o?.value ?? '')}
      itemToStringLabel={(o: FilterOption) => o.label}
      isItemEqualToValue={(a: FilterOption, b: FilterOption) =>
        a.value === b.value
      }
      inputValue={keyword}
      onInputValueChange={setKeyword}
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) setKeyword('');
      }}
    >
      <ComboboxTrigger
        render={
          <Button variant="outline" className="justify-start font-normal" />
        }
      >
        {label}
        <span className="truncate">
          <ComboboxValue />
        </span>
      </ComboboxTrigger>
      <ComboboxContent className="w-auto min-w-(--anchor-width)">
        <ComboboxInput
          showTrigger={false}
          placeholder={t('filter.searchPlaceholder')}
        />
        <ComboboxEmpty>
          {batches.isFetching ? t('filter.loading') : t('filter.noMatch')}
        </ComboboxEmpty>
        <ComboboxList
          ref={listRef}
          onScroll={(e) => {
            const el = e.currentTarget;
            // Fetch the next batch less than one row from the bottom; waiting for the very bottom shows a stretch of
            // blank.
            const nearEnd =
              el.scrollTop + el.clientHeight >= el.scrollHeight - 32;
            if (nearEnd && batches.hasNextPage && !batches.isFetchingNextPage) {
              void batches.fetchNextPage();
            }
          }}
        >
          {(o: FilterOption) => (
            <ComboboxItem key={o.value} value={o}>
              {o.label}
            </ComboboxItem>
          )}
        </ComboboxList>
        {/* Options failed to load: the "All" item is still there and the list is not empty, so the failure goes below
            the list, not into an empty state. */}
        {batches.isError && !batches.data ? (
          <LoadFailed className="py-3" onRetry={() => void batches.refetch()} />
        ) : null}
      </ComboboxContent>
    </Combobox>
  );
}
