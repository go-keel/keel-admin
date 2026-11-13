/**
 * The sidebar collapse state is a **preference**: it follows the device and lives in localStorage (defined in
 * GLOSSARY.md, "The two kinds of choices a person makes"). shadcn's SidebarProvider has its own cookie fallback, but that one
 * follows the domain and cannot be read back; the shell uses localStorage throughout, so this is controlled.
 */
import { readPreference, writePreference } from './preference';

const KEY = 'keel_sidebar_open';

/** Expanded by default: someone arriving for the first time should see the menu first. */
export function getSidebarOpen(): boolean {
  return readPreference(KEY) !== 'false';
}

export function setSidebarOpen(open: boolean): void {
  writePreference(KEY, String(open));
}
