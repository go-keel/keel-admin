/**
 * Which section a form dialog with section navigation shows after a submit fails validation: stay on the current one
 * if it has an error itself (the person is looking at it), otherwise the first section with an error, in navigation
 * order. `invalid` is in navigation order. With no section in error (the failure was elsewhere, such as a missing
 * one-time code), stay.
 */
export function sectionAfterInvalidSubmit(
  current: string,
  invalid: readonly string[],
): string {
  if (invalid.length === 0 || invalid.includes(current)) return current;
  return invalid[0] ?? current;
}
