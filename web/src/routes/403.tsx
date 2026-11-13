import { createFileRoute } from '@tanstack/react-router';
import { Forbidden } from '@/components/exception';

/**
 * 403 is its own page rather than rendered in place inside the shell: once the authenticated shell's beforeLoad decides
 * the page cannot be entered it throws a redirect here, so the leaf route's loader never sends requests that are bound
 * to get 403. "The page lets you in, then gets 403 while loading" is exactly the mismatch to avoid.
 */
export const Route = createFileRoute('/403')({
  component: Forbidden,
});
