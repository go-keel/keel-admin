# Time is passed in three ways by meaning: instants as UTC, half-open; dates as dates only, inclusive at both ends

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

## Context

"September 1" has three meanings in an admin: a span of instants cut by some time zone, a day in business terms, and a
day that belongs to no time zone (a birthday). Passing all three the same way always leaves one of them off by a time
zone or a day: pass a birthday as an instant, and read in another time zone it becomes August 31; pass a creation-time
filter as a date, and the server does not know whose midnight to cut at.

The display time zone in the top bar can be changed at any time. If filters were reinterpreted in the new time zone
the moment it changes, every change would mean refetching: the list flashes back to loading, returns to page 1, and
the selection is cleared.

## Decision

**First identify the meaning, then choose how to pass it** (terms in `GLOSSARY.md`):

| Meaning | How it is passed | Range |
|---|---|---|
| Instant | UTC RFC3339 | Half-open: `>= from`, `< to` |
| Business day | `YYYY-MM-DD`, converted by the server using the business time zone | Inclusive at both ends |
| Calendar date | `YYYY-MM-DD`, not converted | Inclusive at both ends |

To remember it in one sentence: **what is passed as a date is inclusive at both ends; what is passed as an instant is
half-open.**

Ranges in the UI are always inclusive at both ends: dates to the day, times to the second. "Until September 3" and
"until 23:59:59" both include that last day and that last second; when converting to instants, the kernel moves
the end one unit later (sending "before the 4th 00:00", "before the next day 00:00"), and neither business pages nor
endpoints write this step.

Five accompanying rules:

1. **Parameter names carry the meaning**: instant ranges are `<field>_from_at` / `<field>_to_at`, date ranges
   `<field>_from_date` / `<field>_to_date`;
2. **filters are either following or fixed**: following filters (date ranges, time ranges to the second) are
   converted to instants at fetch time using the display time zone at that moment, and the filter is labelled with the
   display time zone; fixed filters are only date ranges, pick business days, and are labelled with the business time
   zone. The URL stores only the date (time), never the time zone;
3. **changing the display time zone only changes how times are displayed; it does not refetch**: following filters
   are converted with the new time zone at the next fetch (paging, refresh, the background refetch when returning to
   the page);
4. **the business time zone is one value for the whole system**: the server's `sys.business_time_zone` is required,
   and startup fails if it is missing or not recognized; after login it is sent through the metadata endpoint and used
   for labels, for how "today" is determined, and as the display time zone's default when nobody has chosen one and
   the browser's time zone is not recognized; it takes no part in request conversion;
5. **the system is uniformly UTC**: the process time zone, the database connection's `loc` and the session
   `time_zone` are all UTC.

## Why dates are inclusive at both ends, instants half-open, and the UI always inclusive

Instants are continuous: writing the end of "until September 3" as `23:59:59` misses the milliseconds in the last
second; only "before September 4 00:00" misses nothing. Dates come in discrete cells: `2026-09-03` in a parameter is
"until September 3" in the UI, readable at a glance in URLs and logs; written as `2026-09-04`, frontend and backend
each have one place that must remember to "add a day", and missing either gives a day too many or too few. "Until
23:59:59" in the UI is the same: the user reads it as including that second; sending "at or before 23:59:59" literally
misses the milliseconds within it, and having every page remember to add a second guarantees some will not, so it is
added once, in the kernel's conversion.

## Why changing the display time zone does not refetch with it right away

"Following" means "as understood in the viewer's current time zone"; the same link already means different data to
people in different time zones. Not refetching at the moment of change only postpones the reinterpretation to the next
fetch, and buys the fewest rules: the URL carries no time zone, and the query key is determined by the URL alone. The
cost is that between the change and the next fetch, the table still holds the rows filtered with the old time zone,
while the filter is already labelled with the new one.

## Why an unrecognized display time zone falls back to the business time zone, after the browser

When nobody has chosen, the viewer's own environment is trusted first: if the browser's location is a registered time
zone, it is used, so people see times where they are. Only when it is not recognized (that time zone is not registered
in the dropdown) does a configured value take over, and then the fallback is the business time zone rather than UTC:
UTC is nobody's "own time", while the business time zone at least matches the times colleagues mention. Deploying in
another region also changes only this one server-side setting, with no frontend change or rebuild. If a derived repo
sets a default in the shell config, that wins and the browser is not consulted.

## Why business days are passed as dates rather than converted by the frontend

The business time zone is a server-side business rule, used by daily aggregation and settlement days. With the server
converting, the rule lives in one place; with the frontend converting, each side holds a copy, and changing one but
forgetting the other is a day off.

## Why both the process and the database are set to UTC

Set only the connection's `loc`, and `DEFAULT CURRENT_TIMESTAMP` still takes its value in the MySQL session time zone,
mixing readings in two time zones within one column; set only the session time zone, and Go reads and writes
`DATETIME` interpreted by `loc`. With both set to UTC, deploying on a machine in a different time zone does not shift
the readings of historical data as a whole.

## Rejected approaches

- **Refetching with the new time zone the moment the display time zone changes**: every change flashes a load,
  returns to page 1 and clears the selection, and even a list with no date filled in has to decide first whether to
  refetch.
- **The filter time zone goes into the URL with the filter value**: refresh and sharing give the same result whoever
  opens it, but the URL carries one more segment, and when the chosen time zone and the top bar's differ, there must be
  a rule for which wins: more rules than it buys.
- **Fixed filters may declare any time zone**: the "fixed time zone" would differ from page to page within one admin,
  and users would have to work it out page by page; add a parameter to fixed filters if a real need arises.
- **The business time zone in the frontend shell config**: a build-time constant, so when the server needs it the same
  value has to be configured in two places.
