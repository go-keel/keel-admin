import { RestrictToVerticalAxis } from '@dnd-kit/abstract/modifiers';
import {
  Accessibility,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/dom';
import { RestrictToElement } from '@dnd-kit/dom/modifiers';
import { move } from '@dnd-kit/helpers';
import { DragDropProvider } from '@dnd-kit/react';
import { useSortable } from '@dnd-kit/react/sortable';
import { GripVerticalIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuItem,
} from '@/components/ui/sidebar';
import { reorderFavorites } from '@/lib/favorites';

/**
 * A favorites group sortable by dragging.
 *
 * **The keyboard must be able to sort too**: Tab focuses the handle, Space picks up, arrow keys move, Space drops. Drag
 * that only understands pointers does not exist for keyboard users, the same principle as the column-width handle (see
 * docs/admin/tables.md). This is also why there are no hand-rolled HTML5 drag events: that route has no keyboard and
 * no touch support.
 *
 * The order is saved to the server as **a whole-list overwrite** (see reorderFavorites in lib/favorites.ts): when
 * dragging, the user really is saying "this is the whole order".
 */
export function SortableFavorites({
  paths,
  render,
}: {
  paths: string[];
  render: (path: string) => ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <DragDropProvider
      // The sidebar is a vertical strip; free horizontal drag would only take the pointer over the content area.
      modifiers={(defaults) => [
        ...defaults,
        RestrictToVerticalAxis,
        RestrictToElement.configure({
          element: (op) => op.source?.element?.parentElement ?? null,
        }),
      ]}
      plugins={(defaults) => [
        ...defaults,
        Accessibility.configure({
          announcements: {
            dragstart: ({ operation }: DragStartEvent) =>
              t('nav.dragStart', { name: String(operation.source?.id) }),
            dragend: ({ operation, canceled }: DragEndEvent) =>
              canceled
                ? undefined
                : t('nav.dragEnd', { name: String(operation.source?.id) }),
          },
        }),
      ]}
      onDragEnd={(event) => {
        if (event.canceled) return;
        const next = move(paths, event);
        if (next.every((path, i) => path === paths[i])) return;
        // No local rollback on failure: the full list returned by the server overwrites it (the middleware has already
        // toasted the error).
        void reorderFavorites(next);
      }}
    >
      <SidebarMenu>
        {paths.map((path, index) => (
          <SortableItem key={path} id={path} index={index}>
            {render(path)}
          </SortableItem>
        ))}
      </SidebarMenu>
    </DragDropProvider>
  );
}

/**
 * Only the handle at the end of the row drags, and it shows only while hovering the item (shadcn showOnHover); the
 * whole item is still a link that opens on click. When the sidebar collapses to icons the handle hides with the
 * component; sorting is then unavailable, expand the sidebar to sort.
 */
function SortableItem({
  id,
  index,
  children,
}: {
  id: string;
  index: number;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const { ref, handleRef } = useSortable({ id, index });

  return (
    <SidebarMenuItem ref={ref}>
      {children}
      <SidebarMenuAction
        ref={handleRef}
        showOnHover
        aria-label={t('nav.dragHandle')}
        className="cursor-grab"
      >
        <GripVerticalIcon />
      </SidebarMenuAction>
    </SidebarMenuItem>
  );
}
