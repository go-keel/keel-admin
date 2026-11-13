import { Link, useRouter, useRouterState } from '@tanstack/react-router';
import { ChevronRightIcon } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { SortableFavorites } from '@/components/sortable-favorites';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  useSidebar,
} from '@/components/ui/sidebar';
import { useProfile } from '@/hooks/use-profile';
import { BRAND_LOGO, brandTitle } from '@/lib/brand';
import { favoritePaths } from '@/lib/favorites';
import { tDynamic } from '@/lib/i18n';
import {
  buildMenu,
  FAVORITES_KEY,
  type MenuFolder,
  type MenuGroup,
  type MenuItem,
  type MenuPage,
} from '@/lib/nav-model';
import { currentPageOf, routeLayersOf } from '@/lib/route-meta';
import { nav } from '@/nav';
import { shell } from '@/shell';

/**
 * The sidebar only draws: which groups and items to show, which item is current and which group it sits in, all
 * computed by buildMenu in `lib/nav-model.ts` (that side can be unit-tested, this one cannot). The favorite star is not
 * in the sidebar but after the page name in the header (see components/app-header.tsx).
 */

/**
 * Vertical padding of a group. SidebarGroup defaults to `p-2`, so two adjacent groups leave 16px of space, while items
 * within a group have 0 between them; by contrast every group looks like it floats on its own empty patch. Reduced to
 * 4px; the horizontal padding stays.
 */
const GROUP_PADDING = 'py-1';

/**
 * When the sidebar collapses to icons, the component moves SidebarGroupLabel up one row (-mt-8) and makes it
 * transparent, yet it still catches clicks, landing right over the last item of the previous group so that item cannot
 * be clicked. When collapsed, the label ignores clicks.
 */
const GROUP_LABEL = 'group-data-[collapsible=icon]:pointer-events-none';

function PageButton({ item, title }: { item: MenuPage; title: string }) {
  const Icon = item.icon;
  return (
    <SidebarMenuButton
      isActive={item.active}
      tooltip={title}
      render={<Link to={item.path} />}
    >
      {Icon ? <Icon /> : null}
      <span>{title}</span>
    </SidebarMenuButton>
  );
}

/**
 * A grouping parent item. Expanded: the whole row is the toggle handle, the arrow on the right rotates with it, and
 * children are indented below (the structure of shadcn sidebar-07); entering any page in the group expands it
 * automatically, expanding only, never collapsing, and the state is not remembered. The child list hangs in **the
 * same** <li>: a separate SidebarMenuItem would nest an <li> inside an <li>.
 *
 * Collapsed to icons, the component hides the children and clicking the icon opens a child menu on the right instead:
 * the sidebar does not expand, because collapsing was the user's own choice. Then only this icon remains, and it marks
 * the current position when inside the group. On mobile the sidebar is a drawer and draws as expanded.
 */
function FolderMenu({
  item,
  titleOf,
}: {
  item: MenuFolder;
  titleOf: (item: MenuPage) => string;
}) {
  const { state, isMobile } = useSidebar();
  const [open, setOpen] = useState(item.inside);
  // Expand on entering the group: the "adjusting state while rendering" pattern from the React docs, effective in the
  // same pass as the page change.
  const [wasInside, setWasInside] = useState(item.inside);
  if (item.inside !== wasInside) {
    setWasInside(item.inside);
    if (item.inside) setOpen(true);
  }

  const Icon = item.icon;
  const label = tDynamic(item.labelKey, item.labelKey);

  if (state === 'collapsed' && !isMobile) {
    return (
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <SidebarMenuButton tooltip={label} isActive={item.inside} />
            }
          >
            {Icon ? <Icon /> : null}
            <span>{label}</span>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="right" align="start">
            <DropdownMenuGroup>
              <DropdownMenuLabel>{label}</DropdownMenuLabel>
              {item.children.map((child) => {
                const ChildIcon = child.icon;
                return (
                  <DropdownMenuItem
                    key={child.path}
                    render={
                      <Link
                        to={child.path}
                        aria-current={child.active ? 'page' : undefined}
                      />
                    }
                  >
                    {ChildIcon ? <ChildIcon /> : null}
                    <span>{titleOf(child)}</span>
                  </DropdownMenuItem>
                );
              })}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    );
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      render={<SidebarMenuItem />}
    >
      <CollapsibleTrigger render={<SidebarMenuButton tooltip={label} />}>
        {Icon ? <Icon /> : null}
        <span>{label}</span>
        {/* The arrow watches only whether its own button is open (the trigger carries data-panel-open when expanded):
            borrowing the outer container's group name would make it rotate with the collapsible group of the same name.
            */}
        <ChevronRightIcon className="ml-auto transition-transform group-data-panel-open/menu-button:rotate-90" />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <SidebarMenuSub>
          {item.children.map((child) => {
            const ChildIcon = child.icon;
            return (
              <SidebarMenuSubItem key={child.path}>
                <SidebarMenuSubButton
                  isActive={child.active}
                  render={<Link to={child.path} />}
                >
                  {ChildIcon ? <ChildIcon /> : null}
                  <span>{titleOf(child)}</span>
                </SidebarMenuSubButton>
              </SidebarMenuSubItem>
            );
          })}
        </SidebarMenuSub>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function AppSidebar() {
  // Subscribing to language changes is enough: menu titles go through tDynamic with dynamic keys, which t cannot track
  // statically.
  useTranslation();
  const profile = useProfile();
  const router = useRouter();
  const currentPage = useRouterState({
    select: (s) => currentPageOf(s.matches)?.path,
  });
  const favorites = favoritePaths(profile);

  const layersOf = useCallback(
    (path: string) => routeLayersOf(router, path),
    [router],
  );
  const groups = useMemo(
    () =>
      buildMenu(nav, {
        profile,
        layersOf,
        currentPage,
        favorites,
      }),
    [profile, layersOf, currentPage, favorites],
  );

  const titleOf = (item: MenuPage) =>
    item.titleKey ? tDynamic(item.titleKey, item.path) : item.path;

  const menuItem = (item: MenuItem) =>
    item.kind === 'page' ? (
      <SidebarMenuItem key={item.path}>
        <PageButton item={item} title={titleOf(item)} />
      </SidebarMenuItem>
    ) : (
      <FolderMenu key={item.labelKey} item={item} titleOf={titleOf} />
    );

  const renderGroup = (group: MenuGroup) => {
    const label = group.labelKey
      ? tDynamic(group.labelKey, group.labelKey)
      : undefined;
    const body = (
      <SidebarGroupContent>
        {group.key === FAVORITES_KEY ? (
          <SortableFavorites
            paths={group.items.flatMap((item) =>
              item.kind === 'page' ? [item.path] : [],
            )}
            render={(path: string) => {
              const item = group.items.find(
                (i) => i.kind === 'page' && i.path === path,
              );
              return item?.kind === 'page' ? (
                <PageButton item={item} title={titleOf(item)} />
              ) : null;
            }}
          />
        ) : (
          <SidebarMenu>{group.items.map((item) => menuItem(item))}</SidebarMenu>
        )}
      </SidebarGroupContent>
    );

    // Collapsing needs a clickable handle, and the handle is the label: a group without a label cannot collapse.
    return group.collapsible && label ? (
      <Collapsible key={group.key} defaultOpen className="group/collapsible">
        <SidebarGroup className={GROUP_PADDING}>
          <CollapsibleTrigger
            // Rendered as SidebarGroupLabel (a div), not a native <button>; without declaring that, Base UI warns that
            // the button semantics were removed.
            nativeButton={false}
            render={
              <SidebarGroupLabel
                className={`${GROUP_LABEL} hover:bg-sidebar-accent hover:text-sidebar-accent-foreground`}
              />
            }
          >
            {label}
            <ChevronRightIcon className="ml-auto transition-transform group-data-[open]/collapsible:rotate-90" />
          </CollapsibleTrigger>
          <CollapsibleContent>{body}</CollapsibleContent>
        </SidebarGroup>
      </Collapsible>
    ) : (
      <SidebarGroup key={group.key} className={GROUP_PADDING}>
        {label ? (
          <SidebarGroupLabel className={GROUP_LABEL}>{label}</SidebarGroupLabel>
        ) : null}
        {body}
      </SidebarGroup>
    );
  };

  return (
    <Sidebar
      variant={shell.layout.sidebarVariant}
      collapsible={shell.layout.sidebarCollapsible}
    >
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            {/* Same height as menu items, so the top of the sidebar is as tall as the header on the right. The logo is
                the same size as menu icons; the rule by which buttons size icons only applies to svg, and the logo is
                an img it does not reach, so the icon size is written out again here. */}
            <SidebarMenuButton render={<Link to="/" />}>
              <img
                src={BRAND_LOGO}
                alt=""
                className="size-4 shrink-0 object-contain"
              />
              <span className="truncate font-semibold text-base">
                {brandTitle()}
              </span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>{groups.map(renderGroup)}</SidebarContent>
    </Sidebar>
  );
}
