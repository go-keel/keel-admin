# Form initial values always come fresh from the detail endpoint: writing back stale data is data loss, not lag

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

When an edit form opens, its initial values must be fetched fresh from the detail endpoint (`GetXxx`); they **must
not** come from list results, global state prefetched at the entry point, or any frontend cache. The accompanying
constraint: list endpoints send only the fields the list uses.

## Why

List results and prefetched global state are snapshots of some moment, with no synchronization at all to "what the
database holds now". Once something has been changed elsewhere (another browser tab, another administrator, the same
user on another page), they are stale.

Prefilling a form with stale data does more than "show an old value": the user sees the old value, does not intend to
touch that field, submits, and the old value is written back verbatim as "the user's input", silently overwriting
someone else's change. **Reading stale data is a display problem; writing it back is data loss.** Set fields such as
`roleIds` are especially dangerous (one element fewer = one role removed), and there is no error at all at the
scene, only a user saying "I didn't touch it" and a perfectly normal audit log entry.

The criterion is "is this data sent back on submit", not "does it look fresh":

- pure display (the avatar and nickname in the top bar, list rows) may use a snapshot; staleness is only display lag
  and a refresh fixes it;
- every field that goes into a form and is sent back on submit must be fetched fresh when the form opens.

So **every editable resource must have a detail endpoint**. Without one, do not build an edit entry point. "Add the
dialog first and take the values from the list for now" is the most common way this rule gets broken.

## Why it also constrains list fields

"List endpoints send only the fields the list uses" looks like a matter of performance and security: extra fields
bloat every row of the response and spread things meant only for the detail endpoint (security settings such as
`ip_whitelist`) to everyone who can see the list. But the subtlest cost is that **when a field is at hand, the next
person naturally uses it to prefill a form**, and the rule above breaks on the spot. So both rules must stand
together: if the list shows "has an IP restriction", send a bool, not the whole allowlist.

## Viewing also goes by ID

The same applies to viewing: a list hands only the ID to external components such as drawers and dialogs, and the
details a component shows are fetched fresh from the detail endpoint by that ID, not taken from the list row.
Otherwise "what the drawer shows" ends up deciding what the list sends: an audit log drawer that takes the request
body from the list row makes every row of the list carry a whole request JSON.

Two exceptions: inline edits (changing one value of this row in place); and a name used only to identify the object,
in a confirmation prompt or a title like "Delete X" or "Create under X", where the action itself sends only the ID. A
stale name only makes the prompt text old and cannot hit the wrong object; an extra request and a loading state in
the confirmation box are not worth it.

## Rejected alternatives

"Add a timestamp/version to list results and fetch again when stale": the criterion turns from "is it sent back" into
"is it fresh", and "fresh" is always relative: someone can change it between any two requests. Also rejected:
dirty checking of the "send back only the fields the user changed" kind. It depends on the frontend computing the diff
correctly; once the comparison of a set field is wrong, it breaks exactly as now, only harder to trace.
