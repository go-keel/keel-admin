# User settings are stored on the server and written item by item; device-bound preferences stay in the browser

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

## Context

Users make two kinds of choices that both look like "personal settings" but belong in entirely different places:

- which menus they added to favorites: these should still be there on another computer;
- color mode, appearance preset, time zone, language, whether the sidebar is collapsed: these need not follow to
  another computer, and **the login page has to render correctly with them before any identity is known**.

Putting both in one place necessarily breaks one side: all on the server, and the login page has nothing to read at
that moment; all in the browser, and favorites cannot follow the person.

## Decision

**The two kinds are handled separately, each with its own name** (terms in each repository's `GLOSSARY.md`):

- **preferences** follow the device, are stored locally in the browser, and the server knows nothing of them;
- **user settings** follow the account, are stored in a column of the user row, and are sent along with the identity
  information.

Three shape constraints on user settings:

1. **One `TEXT` column holding the JSON of a structured object**, encoded and decoded on the server-language side;
   not the database's JSON column type (reasons in `docs/practices/database.md`, "Structured data");
2. **field numbers are split into ranges**: the low range belongs to the kernel, the high range to derived repos'
   additions; parsing ignores unknown fields;
3. **writes are item by item, never a whole-object overwrite**.

## Why writes are item by item

Sending the whole object back (one `UpdateMySettings` taking every field) needs the fewest endpoints, but it loses
data silently: two browsers open at once each change one item, and the later submission overwrites with its own old
snapshot, so the earlier item is gone. **And the user cannot tell what was lost**: both places showed success.

Writing item by item ("add one", "remove one", one endpoint each) + the server locking the user row in a transaction
and doing read-modify-write keeps both. The cost is that the number of endpoints grows with the settings, a
trade-off with a clear price: one or two write endpoints per setting, in exchange for a whole class of data loss
nobody can detect.

Two other shapes do not change this rule:

- **Ordering** is a total-order PUT: the user really is saying "this is the whole order", so overwriting is right.
  Drag-to-reorder of favorites is implemented this way, but it accepts only a **permutation** of the current set:
  extra items are rejected, and items another browser just added that this request does not carry stay at the end
  unchanged. A total order overwrites the order, not the set; otherwise it would become a whole-object write endpoint
  that bypasses item-by-item writes and lets the data loss above back in.
- **Scalar settings** use an update with a field mask: the mask says "I am changing only this one", so it is still not
  a whole-object overwrite.

## What does not go into user settings

- what the server needs to query across users (that needs indexing, so a column or table of its own);
- what is security-related and must be audited (that needs its own audit home and gate);
- data of the business system (that belongs in the business's own tables).

User settings' write endpoints are deliberately **neither audited nor step-up**: they change only the arrangement of
the user's own interface, touching neither permissions nor anyone else's data. Recording them in the audit log would
only bury the records that actually need to be checked.

## Consequences

- As settings grow, the list of endpoints grows longer. When that day comes, converge on the two shapes above
  (ordering / scalar), never back to whole-object overwrites.
- A JSON text column cannot be queried or indexed. That is precisely the boundary drawn in "What does not go into user
  settings": anything that needs querying should not go in to begin with.
