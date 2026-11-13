/**
 * Read/write access for preferences. Preferences are the axes that follow the device and live in browser localStorage
 * (color mode, appearance preset, time zone, language, sidebar collapse), defined in GLOSSARY.md, "The user's two kinds
 * of choices".
 *
 * try/catch lives only here: in private mode localStorage may be entirely unavailable (reading throws); an unreadable
 * value counts as "never stored" and resolution continues, and a failed write must not undo this switch: it takes
 * effect on this page as usual, only another device / the next visit gets the resolved value. With one copy per axis,
 * this convention relied on memory alone.
 */
export const readPreference = (key: string): string | null => {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
};

export const writePreference = (key: string, value: string): void => {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Without persistence, the change still takes effect on this page immediately.
  }
};
