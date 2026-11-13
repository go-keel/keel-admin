import { useQuery } from '@tanstack/react-query';
import { EyeIcon, EyeOffIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActionDialog } from '@/components/containers';
import { DataRegion } from '@/components/data-region';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useProfile } from '@/hooks/use-profile';
import { clearPreview, getPreview, setPreview } from '@/lib/preview';
import { roleOptionsQueryOptions } from '@/lib/query';

/**
 * Reduced-permission preview (root): after selecting some roles the page fully reloads, and every request then carries
 * X-Preview-Roles; the backend reduces root to the permission view of those roles (only reduces, never raises). The
 * reload is required: the profile, the menu and every cached list must restart with the new identity; partial
 * invalidation is more likely to miss something.
 *
 * The entry shows only for root, **but the exit cannot be gated on root too**: while preview is active, the profile's
 * isRoot already reflects the reduced identity (the server uses the principal), so gating on root would hide the exit
 * button as well and trap the person in preview. So the gate is "is there a local preview state".
 *
 * A popover rather than a dropdown menu: this is "tick some items, take a look, then decide", whereas every menu item
 * is an action, not a form you can change back and forth. Entering and exiting each ask for confirmation, because both
 * fully reload the page and a slip means starting over.
 */
export function PreviewMenu() {
  const profile = useProfile();
  const preview = getPreview();

  if (preview) return <PreviewExit roles={preview.names} />;
  if (!profile.isRoot) return null;
  return <PreviewPicker />;
}

function PreviewExit({ roles }: { roles: string[] }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Tooltip>
        <TooltipTrigger
          render={<Button variant="ghost" onClick={() => setOpen(true)} />}
        >
          <EyeOffIcon />
          <span className="max-w-32 truncate">
            {t('preview.active', {
              roles: roles.join(t('format.listSeparator')),
            })}
          </span>
        </TooltipTrigger>
        <TooltipContent>{t('preview.exit')}</TooltipContent>
      </Tooltip>
      <ActionDialog
        open={open}
        onOpenChange={setOpen}
        title={t('preview.exit')}
        description={t('preview.exitConfirm')}
        confirmLabel={t('preview.exit')}
        onConfirm={() => {
          clearPreview();
          window.location.reload();
        }}
      />
    </>
  );
}

function PreviewPicker() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  // Role options are only needed when root opens the popover, so they are not in the shell prefetch.
  const query = useQuery({ ...roleOptionsQueryOptions, enabled: open });
  const roles = query.data?.items ?? [];
  // **Several** roles can be selected: a person usually holds several roles, and previewing only one would hide the
  // real permission surface they add up to. X-Preview-Roles is a CSV anyway.
  const [picked, setPicked] = useState<string[]>([]);

  const enter = () => {
    if (picked.length === 0) return;
    setPreview({
      ids: picked.join(','),
      names: roles
        .filter((r) => picked.includes(r.id ?? ''))
        .map((r) => r.name ?? ''),
    });
    window.location.reload();
  };

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        // Closing means nothing was chosen: keeping the last ticks would make the next open look "already chosen",
        // which is not what the user just expressed.
        if (!next) setPicked([]);
      }}
    >
      <PopoverTrigger
        render={
          <Button variant="ghost" size="icon" aria-label={t('preview.label')} />
        }
      >
        <EyeIcon />
      </PopoverTrigger>
      {/* Height limit and scrolling sit on the popover, not on the role list: Base UI's Checkbox has an internal
          element that overflows its own border by about 5px, so once the list has its own overflow-y-auto, two roles
          can scroll out a ghost scrollbar. The popover's p-2.5 absorbs exactly that overflow. */}
      <PopoverContent
        align="end"
        className="max-h-(--available-height) w-64 overflow-y-auto"
      >
        <div className="flex flex-col gap-3">
          <PopoverTitle>{t('preview.enter')}</PopoverTitle>
          <DataRegion query={query}>
            {() =>
              roles.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  {t('preview.empty')}
                </p>
              ) : (
                <div className="flex flex-col gap-2">
                  {roles.map((role) => {
                    const id = role.id ?? '';
                    return (
                      <Label
                        key={id}
                        className="flex items-center gap-2 font-normal text-sm"
                      >
                        <Checkbox
                          checked={picked.includes(id)}
                          onCheckedChange={(checked) =>
                            setPicked((prev) =>
                              checked
                                ? [...prev, id]
                                : prev.filter((x) => x !== id),
                            )
                          }
                        />
                        <span className="truncate">{role.name}</span>
                      </Label>
                    );
                  })}
                </div>
              )
            }
          </DataRegion>
          <Button
            size="sm"
            disabled={picked.length === 0}
            onClick={enter}
            className="w-full"
          >
            {t('preview.start', { count: picked.length })}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
