import { Link, useNavigate } from '@tanstack/react-router';
import { LogOutIcon, UserIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useProfile } from '@/hooks/use-profile';
import { logout } from '@/lib/query';

/** Account menu. This is the only entry point; the bottom of the sidebar has no second one. */
export function UserMenu() {
  const { t } = useTranslation();
  const profile = useProfile();
  const navigate = useNavigate();
  const name = profile.nickname || profile.username || '';
  // With a nickname, the second line adds the username: nicknames can repeat, usernames cannot.
  const username = profile.nickname ? profile.username : undefined;

  return (
    <DropdownMenu>
      {/* The avatar itself is the trigger, with no button around it (as in the reference shadcn admin): wrapping it in
          a ghost button shows a ring of background around the avatar on hover. Size uses the component default, the
          same as the header icon buttons already, so nothing extra is written. */}
      <DropdownMenuTrigger
        nativeButton={false}
        aria-label={name}
        render={<Avatar />}
      >
        <AvatarFace name={name} src={profile.avatar} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        <div className="flex items-center gap-2 px-1 py-1.5">
          <Avatar>
            <AvatarFace name={name} src={profile.avatar} />
          </Avatar>
          <div className="grid min-w-0 flex-1 text-sm leading-tight">
            <span className="truncate font-semibold">{name}</span>
            {username ? (
              <span className="truncate text-muted-foreground text-xs">
                {username}
              </span>
            ) : null}
          </div>
        </div>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem render={<Link to="/sys/profile" />}>
            <UserIcon />
            {t('account.profile')}
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={async () => {
            await logout();
            await navigate({ to: '/login' });
          }}
        >
          <LogOutIcon />
          {t('account.logout')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function AvatarFace({ name, src }: { name: string; src?: string }) {
  return (
    <>
      {src ? <AvatarImage src={src} alt="" /> : null}
      <AvatarFallback>
        {name.slice(0, 1).toUpperCase() || <UserIcon />}
      </AvatarFallback>
    </>
  );
}
