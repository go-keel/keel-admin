import {
  ChevronDownIcon,
  ExternalLinkIcon,
  MoreHorizontalIcon,
} from 'lucide-react';
import { Fragment, type ReactNode, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActionDialog, ConfirmPopover } from '@/components/containers';
import { Button } from '@/components/ui/button';
import { ButtonGroup } from '@/components/ui/button-group';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { inlineCount } from '@/lib/table';
import { cn } from '@/lib/utils';

/**
 * Row actions: what the page writes is what the row shows. Each cell of the actions column sets `kind`, chosen by
 * these three cases:
 *
 * - `button`: a single action the page always shows, or one bound to one permission code;
 * - `menu`: a set of actions belonging to one business domain. The button text is the set's group name ("Funds ⌄"),
 *   not a specific action; without `label` it is a "⋯" icon button. When the user has no permission for any item in
 *   the set, the cell is not rendered, so the page need not check for empty;
 * - `split`: like `menu`, but the action marked `primary` moves to the left half as a direct button, and the ⋯ on the
 *   right half holds the rest. **Without an item marked `primary` the whole cell is drawn as a `menu`**: when the
 *   primary action has no permission, the second one does not move up, because the column width is fixed and names
 *   that move up vary in length, so the column would need space for the longest. A dangerous action is not moved up
 *   even when marked (putting delete or disable in the handiest place should be avoided anyway); if it is the only
 *   one left after moving up, it is an ordinary button.
 *
 * The choice comes down to one test: **can these actions get a group name the user understands at a glance**. If not
 * (only "More" or "Other" fits), they are not a group: give each its own `button` cell, and leave what does not fit
 * to `maxInline`. What none of the three can express (a switch, a status badge, a progress bar) is written as JSX in
 * `cell` directly, and the kernel promises nothing about that column; but the container rules are site-wide, and a
 * hand-written column is still bound by them.
 *
 * A menu item is either an action or a named set of actions (`RowActionGroup`, which may nest). **Flatten when it
 * can be flattened, nest when it cannot**: groups at the menu's first level are laid flat as "title + those items",
 * deeper ones become submenus; when `maxInline` folds this cell into "⋯", everything drops one level and the formerly
 * flat level uses submenus: two levels of titles on one plane do not show which belongs to which. The trade-off is in
 * `docs/admin/tables.md`, "Actions column".
 *
 * The action itself (the same whether shown on a button or tucked into a menu) is one of three:
 * - `onSelect` is an in-place action (open a drawer, open a dialog);
 * - `confirm` is an action that needs confirming first: when its button sits directly on the row, a popover anchored
 *   to it confirms; inside a menu a dialog confirms instead, because once the menu closes the popover has no button to
 *   anchor to (the container rules). This split happens once, here; the page gives only the text and callback;
 * - `href` **goes to the list related to this row** with the filter values set. It must be a real link, not
 *   `window.open`: middle-click, Cmd+click and right-click "copy link" are what a user rightly expects from a link,
 *   and simulating it with a click callback takes them all away. An item with href gets a ↗ suffix and opens in a new
 *   browser tab: the current page's filter state should not be replaced.
 */
export type RowAction = {
  key: string;
  label: string;
  /**
   * The icon to the left of the menu item. Whether it is drawn on a **button** is up to the cell: `button` and `menu`
   * draw it when given, `split` needs `showIcon`; icons do not appear on their own, since an icon on a button means
   * the column's width must be measured again.
   */
  icon?: ReactNode;
  onSelect?: () => void;
  /** The address of the related list (with filter values). */
  href?: string;
  confirm?: {
    title: string;
    description?: string;
    needOtp?: boolean;
    /**
     * The sentence to say after success; do not give it for actions with a visible change on screen (see
     * ActionDialog). Fixed text only: the actions of a row share one array with differing return types, which cannot
     * match a toast function's argument; an action whose message depends on what it returns uses `onSelect` to open
     * an ActionDialog.
     */
    success?: string;
    onConfirm: (otpCode: string) => Promise<void>;
  };
  /**
   * The color of the button on the row; a plain outline when omitted. `positive` is the one that moves the queue a
   * step forward (confirm, approve), at most one per row; `destructive` is a dangerous action, and the confirm button
   * of its confirmation turns red with it.
   */
  tone?: 'positive' | 'destructive';
  /**
   * This item is the primary action of its `split`, moved to the left half button. Only the first marked one in a cell
   * counts; after the page filters by permission it may not be in the array at all, and the cell then falls back to a
   * `menu` by itself. The kernel cannot see filtered-out items, and without this mark could not tell whether the first
   * item is the primary action or one that moved up.
   */
  primary?: boolean;
  /**
   * Unavailable for this row right now: the button on the row and the menu item are disabled and cannot be clicked,
   * and `onSelect` / `href` / `confirm` do not fire. The page passes availability as decided by the server; the kernel
   * does not infer it.
   */
  disabled?: boolean;
  /**
   * Why it is unavailable, shown only with `disabled`: as a hover tooltip on the row's button, as a line under the name
   * in a menu item.
   */
  disabledReason?: string;
};

/** A named set of actions, which may nest. */
export type RowActionGroup = {
  key: string;
  label: string;
  items: RowActionNode[];
};

/** One menu item: an action, or a named set of actions. */
export type RowActionNode = RowAction | RowActionGroup;

/** One cell of the actions column. */
export type RowActionItem =
  | ({ kind: 'button' } & RowAction)
  | {
      kind: 'menu';
      key: string;
      /** Group name. Without it, rendered as a "⋯" icon button. */
      label?: string;
      /** The icon left of the group name; the unnamed "⋯" ignores it, being an icon itself. */
      icon?: ReactNode;
      items: RowActionNode[];
    }
  | {
      kind: 'split';
      key: string;
      /**
       * Group name. The left half button shows the name of the action moved up; the group name serves as the section
       * title when folded into "⋯".
       */
      label: string;
      /**
       * Whether the left half button draws that action's icon. Off by default: icons on buttons are heavy and noisy
       * down a whole column, and drawing them means measuring the column width again. None of the kernel's system
       * pages turns it on; icons appear only in menus.
       */
      showIcon?: boolean;
      items: RowActionNode[];
    };

function isGroup(n: RowActionNode): n is RowActionGroup {
  return 'items' in n;
}

/**
 * Recursively removes empty groups. After the page filters item by item by permission, a group may have nothing left,
 * and it may itself sit inside another group; looking only at the first level would leave an entry that opens to just
 * a title, or an empty submenu.
 */
function prune(nodes: RowActionNode[]): RowActionNode[] {
  return nodes.flatMap<RowActionNode>((n) => {
    if (!isGroup(n)) return [n];
    const items = prune(n.items);
    return items.length > 0 ? [{ ...n, items }] : [];
  });
}

export function RowActions({
  actions,
  maxInline,
}: {
  actions: RowActionItem[];
  /**
   * The most cells this column holds, **with the unnamed "⋯" at the end counting as one**: if everything fits, all
   * are laid flat; if not, the last cell goes to "⋯" and the rest fold into it. The column width is therefore exactly
   * this many cells wide, so measuring the column width and writing this number are the same thing. **Without it
   * everything is laid flat**: how many fit is this table's business, not the kernel's. Write it only when the number
   * of cells varies with permissions and a narrow table cannot hold them; when unsure, leave it out.
   */
  maxInline?: number;
}) {
  // The item tucked into a menu that needs confirming: falls back to a dialog for confirmation.
  const [pending, setPending] = useState<RowAction>();
  // After filtering item by item by permission nothing may be left: first recursively remove empty groups, then check
  // whether the cell has anything left. An empty cell is not rendered, so no button opens to nothing; the flattening,
  // folding and split's primary action below all use this cleaned-up copy.
  const shown = actions.flatMap<RowActionItem>((a) => {
    if (a.kind === 'button') return [a];
    const items = prune(a.items);
    return items.length > 0 ? [{ ...a, items }] : [];
  });
  if (shown.length === 0) return null;
  const inline = shown.slice(0, inlineCount(shown.length, maxInline));
  const folded = shown.slice(inline.length);

  return (
    <div className="flex items-center gap-1">
      {inline.map((a) => (
        <InlineItem key={a.key} item={a} onPending={setPending} />
      ))}
      {folded.length > 0 ? (
        <ActionMenu>
          <FoldedBody items={folded} onPending={setPending} />
        </ActionMenu>
      ) : null}
      {pending?.confirm ? (
        <ActionDialog
          open
          onOpenChange={(next) => {
            if (!next) setPending(undefined);
          }}
          title={pending.confirm.title}
          description={pending.confirm.description}
          needOtp={pending.confirm.needOtp}
          destructive={pending.tone === 'destructive'}
          confirmLabel={pending.label}
          success={pending.confirm.success}
          onConfirm={(otp) => pending.confirm?.onConfirm(otp)}
        />
      ) : null}
    </div>
  );
}

/** How one cell looks on the row. */
function InlineItem({
  item: a,
  onPending,
}: {
  item: RowActionItem;
  onPending: (a: RowAction) => void;
}) {
  if (a.kind === 'button') return <ActionButton action={a} showIcon />;
  // split moves up the action marked primary: only at the top level (groups cannot be clicked), and not a dangerous
  // one. When none can be moved up it is the same as menu: a named set of actions, drawn as one menu.
  const at =
    a.kind === 'split'
      ? a.items.findIndex(
          (n) => !isGroup(n) && n.primary && n.tone !== 'destructive',
        )
      : -1;
  const primary = at < 0 ? undefined : (a.items[at] as RowAction);
  const rest = primary ? a.items.filter((_, i) => i !== at) : a.items;
  if (!primary) {
    return (
      <ActionMenu label={a.label} icon={a.kind === 'menu' ? a.icon : undefined}>
        <MenuNodes nodes={rest} onPending={onPending} />
      </ActionMenu>
    );
  }
  const button = (
    <ActionButton
      action={primary}
      showIcon={a.kind === 'split' && a.showIcon}
    />
  );
  if (rest.length === 0) return button;
  return (
    <ButtonGroup>
      {button}
      <ActionMenu>
        <MenuNodes nodes={rest} onPending={onPending} />
      </ActionMenu>
    </ButtonGroup>
  );
}

/**
 * The cells `maxInline` folds into "⋯": one section per cell, headed by its group name, its contents one level down.
 */
function FoldedBody({
  items,
  onPending,
}: {
  items: RowActionItem[];
  onPending: (a: RowAction) => void;
}) {
  return items.map((a, i) => {
    // Consecutive single actions sit together; each group is its own section.
    const prev = items[i - 1];
    const sep =
      prev !== undefined && (a.kind !== 'button' || prev.kind !== 'button');
    return (
      <Fragment key={a.key}>
        {sep ? <DropdownMenuSeparator /> : null}
        {a.kind === 'button' ? (
          <MenuAction action={a} onPending={onPending} />
        ) : (
          // The title must be inside the Group: Base UI's GroupLabel connects aria-labelledby through the group's
          // context, and outside it throws MenuGroupContext is missing.
          <DropdownMenuGroup>
            {a.label ? <DropdownMenuLabel>{a.label}</DropdownMenuLabel> : null}
            <MenuNodes nodes={a.items} nested onPending={onPending} />
          </DropdownMenuGroup>
        )}
      </Fragment>
    );
  });
}

/**
 * Menu contents. Groups are laid flat as "title + those items"; once `nested` (already under a title, or this cell
 * folded into "⋯" and taking the section title) they become submenus: two levels of titles on one plane do not show
 * which governs which items.
 */
function MenuNodes({
  nodes,
  nested,
  onPending,
}: {
  nodes: RowActionNode[];
  nested?: boolean;
  onPending: (a: RowAction) => void;
}) {
  return nodes.map((n, i) => {
    const prev = nodes[i - 1];
    const sep = prev !== undefined && (isGroup(n) || isGroup(prev));
    return (
      <Fragment key={n.key}>
        {sep ? <DropdownMenuSeparator /> : null}
        {!isGroup(n) ? (
          <MenuAction action={n} onPending={onPending} />
        ) : !nested ? (
          // The title is inside the Group, for the same reason as in FoldedBody.
          <DropdownMenuGroup>
            <DropdownMenuLabel>{n.label}</DropdownMenuLabel>
            <MenuNodes nodes={n.items} nested onPending={onPending} />
          </DropdownMenuGroup>
        ) : (
          <DropdownMenuSub>
            <DropdownMenuSubTrigger>{n.label}</DropdownMenuSubTrigger>
            <DropdownMenuSubContent>
              <MenuNodes nodes={n.items} nested onPending={onPending} />
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        )}
      </Fragment>
    );
  });
}

/** One action in a menu. One that needs confirming is handed back up to confirm in a dialog. */
function MenuAction({
  action: a,
  onPending,
}: {
  action: RowAction;
  onPending: (a: RowAction) => void;
}) {
  if (a.disabled) {
    return (
      <DropdownMenuItem
        disabled
        className={cn(a.tone === 'destructive' && 'text-destructive')}
      >
        {a.icon}
        {a.disabledReason ? (
          <span className="flex flex-col">
            {a.label}
            <span className="text-xs text-muted-foreground">
              {a.disabledReason}
            </span>
          </span>
        ) : (
          a.label
        )}
      </DropdownMenuItem>
    );
  }
  if (a.href) {
    return (
      <DropdownMenuItem
        render={<a href={a.href} target="_blank" rel="noopener" />}
      >
        {a.icon}
        {a.label}
        <ExternalLinkIcon />
      </DropdownMenuItem>
    );
  }
  return (
    <DropdownMenuItem
      onClick={a.confirm ? () => onPending(a) : a.onSelect}
      className={cn(a.tone === 'destructive' && 'text-destructive')}
    >
      {a.icon}
      {a.label}
    </DropdownMenuItem>
  );
}

/**
 * One action shown on the row. Dangerous ones use the built-in destructive variant (light red background, red text),
 * positive ones swap the outline for green text and a green border, the rest are outlined. Positive gets no background:
 * a column of a dozen rows with two or three buttons each would be noisy in full color; it only needs to stand apart
 * from its neighbours. Green is `--success`, not the brand preset, for the same reason as in `StatusBadge`.
 */
function ActionButton({
  action: a,
  showIcon,
}: {
  action: RowAction;
  /** Off by default: unspecified means not drawn, so a new call site added someday does not forget to turn it off. */
  showIcon?: boolean;
}) {
  const variant = a.tone === 'destructive' ? 'destructive' : 'outline';
  const className =
    a.tone === 'positive'
      ? 'border-success/40 text-success hover:text-success dark:border-success/40'
      : undefined;
  // data-icon makes Button narrow the padding on the icon side (a shadcn convention); icon is any node, so the
  // attribute goes on a wrapping span, and contents creates no extra box.
  const icon =
    showIcon && a.icon ? (
      <span className="contents" data-icon="inline-start">
        {a.icon}
      </span>
    ) : null;
  if (a.disabled) {
    const button = (
      <Button variant={variant} className={className} size="sm" disabled>
        {icon}
        {a.label}
      </Button>
    );
    if (!a.disabledReason) return button;
    // A disabled button receives no mouse events, so the tooltip goes on the wrapping layer (shadcn's way to add a
    // tooltip to a disabled button). The radius belongs to this layer and the button inherits it: split's ButtonGroup
    // removes this layer's radius, and without inheriting, the seam between a disabled primary action and the "⋯" on
    // its right would show a gap.
    return (
      <Tooltip>
        <TooltipTrigger
          render={
            <span className="inline-flex w-fit rounded-lg *:rounded-[inherit]!" />
          }
        >
          {button}
        </TooltipTrigger>
        <TooltipContent>{a.disabledReason}</TooltipContent>
      </Tooltip>
    );
  }
  if (a.href) {
    return (
      <Button
        variant={variant}
        className={className}
        size="sm"
        nativeButton={false}
        render={<a href={a.href} target="_blank" rel="noopener" />}
      >
        {icon}
        {a.label}
        <ExternalLinkIcon data-icon="inline-end" />
      </Button>
    );
  }
  if (a.confirm) {
    return (
      <ConfirmPopover
        trigger={
          <Button variant={variant} className={className} size="sm">
            {icon}
            {a.label}
          </Button>
        }
        title={a.confirm.title}
        description={a.confirm.description}
        needOtp={a.confirm.needOtp}
        destructive={a.tone === 'destructive'}
        confirmLabel={a.label}
        success={a.confirm.success}
        onConfirm={a.confirm.onConfirm}
      />
    );
  }
  return (
    <Button
      variant={variant}
      className={className}
      size="sm"
      onClick={a.onSelect}
    >
      {icon}
      {a.label}
    </Button>
  );
}

/** The menu shell: with a group name it is "Funds ⌄", without one a "⋯" icon button. */
function ActionMenu({
  label,
  icon,
  children,
}: {
  label?: string;
  icon?: ReactNode;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            size={label ? 'sm' : 'icon-sm'}
            aria-label={label ? undefined : t('table.moreActions')}
          />
        }
      >
        {label && icon ? (
          <span className="contents" data-icon="inline-start">
            {icon}
          </span>
        ) : null}
        {label}
        {label ? (
          <ChevronDownIcon data-icon="inline-end" />
        ) : (
          <MoreHorizontalIcon />
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">{children}</DropdownMenuContent>
    </DropdownMenu>
  );
}
