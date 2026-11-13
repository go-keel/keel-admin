/**
 * Page header: optional description plus the primary actions on the right. The page name is in the header bar (it reads
 * the route titleKey), not here; a page with neither does not use this. Page layout rules: docs/admin/pages.md, "Page
 * layout".
 */
export function PageHeader({
  description,
  actions,
}: {
  description?: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="mb-4 flex items-start justify-between gap-4">
      {description ? (
        <p className="text-muted-foreground text-sm">{description}</p>
      ) : null}
      {actions ? <div className="ml-auto flex gap-2">{actions}</div> : null}
    </div>
  );
}
