/**
 * en-US resources are declared `as const`, so keys and interpolation variables are literal types: a page that misspells
 * a key or omits a {{variable}} fails tsc on the spot. But then the values are literals too, and another language using
 * `typeof enUS` directly would have to match word for word. This mapped type keeps the structure and frees the values.
 */
export type Translations<T> = {
  [K in keyof T]: T[K] extends string ? string : Translations<T[K]>;
};

/**
 * Optional version of `Translations<T>`: kernel supplementary translations may miss keys, which fall back through
 * i18next's `fallbackLng: 'en-US'`. Used by the `languages.ts` registry and by a derived repo's own supplementary
 * translations.
 */
export type DeepPartial<T> = {
  [K in keyof T]?: T[K] extends string ? string : DeepPartial<T[K]>;
};
