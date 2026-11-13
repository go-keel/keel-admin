# A headless frontend stack: a re-skinnable shell hands derived repos component shape and look separately

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

## Context

What the template gives derived repos is a **re-skinnable shell**: the same admin, looking different for different
brands. An all-in-one component library (components shipped together with look and layout) leaves only one seam for
this: theme variables. Everything beyond the variables (the layout skeleton, component shape, density) is fixed by the
library; a derived repo wanting "its own look" can only hack the kernel, and once hacked it can no longer follow
upgrades.

The second motive is extension: an all-in-one framework's plugin system and implicit conventions (global state at the
entry point, an access plugin, a request plugin) tie business code to a whole package; replacing any piece means
replacing the whole.

## Decision

**The frontend uses a headless stack**: component source is in the repository (third-party components written by the
CLI, read-only, never edited by hand), routing and the data layer are independent of each other, and the look is
decided by a named bundle of CSS variables (an **appearance preset**).

Two hard constraints come with it; without either, the re-skinnability headless brings is undermined by the
template's own pages:

1. **The default preset is the component library's official neutral preset**; the template does not choose a brand
   color for derived repos;
2. **the template's own pages use only semantic tokens** (`bg-background`, `text-muted-foreground` and the like), never
   custom colors.

## Why both constraints are needed

Headless only hands layout and component shape back to the derived repo; it does not make the result re-skinnable by
itself. Once the template's own pages carry their own colors and decorations, a derived repo changing its brand has
to edit page by page, which is more work than adjusting theme variables in an all-in-one library. The criticism
"overbearing, too much styling" then holds on the spot, and choosing headless costs more than it gains.

The problem is not headless itself but **the template overstepping into decisions about the look**. The two
constraints target exactly that: with a neutral default preset and pages using only semantic tokens, changing the
brand shrinks to "swap one preset file"; headless in turn hands layout and component shape to the derived repo. Only
when both hold does the template have the shape it should.

## Rejected alternatives

- **An all-in-one component library with deeper theme customization**: nothing beyond the variables can be changed,
  and the layout skeleton is still someone else's. Re-skinnability has a ceiling on it, and that is the very reason the
  template exists.
- **Both side by side (all-in-one library in the kernel, headless in the business area)**: two sets of components, two
  themes and two form conventions in one admin; derived repos learn everything twice, and upgrades must follow both
  chains.
- **Headless only for the table layer**: the table is indeed the most complex part, but the ceiling on customizing
  the look is in the component library; changing only the table does not solve "the brand color can only be adjusted
  through variables".

## Consequences

- **Changing the style means reinstalling all component source**, not changing a config value: the component source
  is in the repository, and changing the skeleton means generating it again. So the style is a build-time constant,
  written in `docs/admin/shell.md`, "shadcn style and skill".
- More of the component UI has to be written by hand (headless gives state, not UI), which makes the shared helpers
  more important: one mistake there and every list page is wrong. What goes into shared helpers and what is explicitly
  not done is in `docs/admin/tables.md`, "What the shared table parts do and do not do".

## For derived repos

Appearance presets, the menu list and shell switches are all assembly seams; a derived repo changes them without
touching the kernel. **Do not write brand colors into the template's own pages**: that turns changing the brand back
into editing page by page.
