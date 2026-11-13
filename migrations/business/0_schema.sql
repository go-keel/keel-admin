-- Complete schema of the business database, separate from the sys_* tables of the admin database (see
-- migrations/admin/): this file runs against the business database that database.default in config.yaml points to;
-- both databases are required. Directories, baseline and incremental scripts: docs/guides/domain-model.md, "Schema".
--   mysql -h127.0.0.1 -uroot -p <business database> < migrations/business/0_schema.sql
-- Data scope filtering (permdata.ApplyScope) applies only to the business tables' own owner columns, with no
-- cross-database JOIN, so two databases and one (both sources set to the same DSN) behave the same.

-- The mysql client's default character set is not always utf8mb4 (often latin1); without this, non-ASCII text in the
-- script would be stored garbled.
SET NAMES utf8mb4 COLLATE utf8mb4_0900_ai_ci;

-- Example: delete after forking (up to the end of this CREATE TABLE)
-- notice: notices, the example business module (internal/biz|data|service, business area). owner_user_id /
-- owner_dept_id are permdata.ApplyScope's default owner columns (contrast a non-default column mapping such as
-- created_by/dept_id, which needs an explicit remap with permdata.ApplyScopeWith).
-- category_id links to a category: category grants (explicit assignment, sys_role_resource_grant) gate notice
-- visibility through it (ApplyScopeWith(catSpec, Cols{PK: "category_id"})).
CREATE TABLE IF NOT EXISTS notice (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    title         VARCHAR(128)    NOT NULL,
    content       TEXT            NOT NULL,
    category_id   BIGINT UNSIGNED NOT NULL,
    owner_user_id BIGINT UNSIGNED NOT NULL,
    owner_dept_id BIGINT UNSIGNED NOT NULL,
    created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY idx_category_id (category_id),
    KEY idx_owner_user_id (owner_user_id),
    KEY idx_owner_dept_id (owner_dept_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Example: delete after forking (up to the end of this CREATE TABLE)
-- notice_category: notice categories, the example of a grantable instance of explicit assignment (RESOURCE_GRANT).
-- No owner columns: categories are not isolated by user or department; visibility is decided by the role's category
-- grants (the instance grant rows are in sys_role_resource_grant on the kernel side; this table is only the business
-- entity). Names are unique: categories are options of a multi-select in role configuration, and equal names could
-- not be told apart.
CREATE TABLE IF NOT EXISTS notice_category (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(64)     NOT NULL,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_name (name)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;
