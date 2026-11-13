import { createFileRoute } from '@tanstack/react-router';
import { z } from 'zod';
import { LoginLayout } from '@/login-layout';

/**
 * Login page route (kernel). What the page looks like is decided by the assembly seam `web/src/login-layout.tsx`; the
 * login form and flow are in `components/login-form.tsx`.
 */
export const Route = createFileRoute('/login')({
  validateSearch: z.object({ redirect: z.string().optional() }),
  component: LoginLayout,
});
