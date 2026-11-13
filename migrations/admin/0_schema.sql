-- Complete schema of the admin database (MySQL dialect): every sys_* table at its latest shape, matching
-- internal/core/sys/data/*_model.go. Part of the kernel: following upstream overwrites this directory whole, and the
-- ALTER an existing admin database needs is in the matching UPGRADING.md entry (docs/admin/domain-model.md, "Schema").
-- No AutoMigrate in code. Run it against the database database.admin in config.yaml points to (business tables are in
-- migrations/business/, run against the business database; both are required):
--   mysql -h127.0.0.1 -uroot -p <admin database> < migrations/admin/0_schema.sql

-- The mysql client's default character set is not always utf8mb4 (often latin1); without this, non-ASCII text in the
-- script would be stored garbled.
SET NAMES utf8mb4 COLLATE utf8mb4_0900_ai_ci;

-- Department tree (framework master data, soft delete only). ancestors is the id path from the root to the parent
-- department (such as 0,1,5,), used to expand subtrees.
CREATE TABLE IF NOT EXISTS sys_dept (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    parent_id  BIGINT UNSIGNED NOT NULL DEFAULT 0,  -- 0 = top-level department
    ancestors  VARCHAR(1024)   NOT NULL DEFAULT '',
    name       VARCHAR(64)     NOT NULL,
    sort       INT             NOT NULL DEFAULT 0,
    status     TINYINT(1)      NOT NULL DEFAULT 1,  -- 1 enabled, 0 disabled
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at DATETIME(3)     NULL,
    KEY idx_parent (parent_id),
    KEY idx_deleted_at (deleted_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Admin accounts (framework master data, soft delete only, no delete endpoint).
CREATE TABLE IF NOT EXISTS sys_user (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    username       VARCHAR(64)     NOT NULL,
    password       VARCHAR(128)    NOT NULL,
    nickname       VARCHAR(64)     NOT NULL DEFAULT '',
    -- Contact details: optional, not unique, not used to log in (format validated in the contract, see
    -- api/keel_admin/v1/user.proto).
    email          VARCHAR(254)    NOT NULL DEFAULT '',
    phone          VARCHAR(32)     NOT NULL DEFAULT '',
    dept_id        BIGINT UNSIGNED NOT NULL,
    is_root        TINYINT(1)      NOT NULL DEFAULT 0,
    status         TINYINT(1)      NOT NULL DEFAULT 1,  -- 1 enabled, 0 disabled
    otp_secret_enc VARBINARY(255)  NULL,
    otp_bound      TINYINT(1)      NOT NULL DEFAULT 0,
    otp_bound_at   DATETIME(3)     NULL,
    token_version  INT             NOT NULL DEFAULT 0,
    avatar         MEDIUMTEXT      NULL,
    ip_whitelist   VARCHAR(512)    NOT NULL DEFAULT '',
    -- Personal settings that follow the account (JSON of api.keel_admin.v1.UserSettings). A TEXT type rather than
    -- JSON, see docs/practices/database.md (types); read-modify-write takes a row lock in a transaction, with no JSON_*
    -- functions.
    settings       MEDIUMTEXT      NULL,
    created_at     DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at     DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at     DATETIME(3)     NULL,
    UNIQUE KEY uk_username (username),
    KEY idx_dept (dept_id),
    KEY idx_deleted_at (deleted_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Roles (framework master data, soft delete only; deleting revokes the grants, rows in link tables stay).
CREATE TABLE IF NOT EXISTS sys_role (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code        VARCHAR(64)     NOT NULL,
    name        VARCHAR(64)     NOT NULL,
    `sensitive` TINYINT(1)      NOT NULL DEFAULT 0,  -- 1 = only root can assign
    force_otp   TINYINT(1)      NOT NULL DEFAULT 0,  -- 1 = holders must bind an authenticator
    status      TINYINT(1)      NOT NULL DEFAULT 1,  -- 1 enabled, 0 disabled (a disabled role grants nothing)
    created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at  DATETIME(3)     NULL,
    UNIQUE KEY uk_code (code),
    KEY idx_deleted_at (deleted_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Which roles a user has (link table, replaced as a whole when the user is edited).
CREATE TABLE IF NOT EXISTS sys_user_role (
    user_id BIGINT UNSIGNED NOT NULL,
    role_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (user_id, role_id),
    KEY idx_role (role_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Permission codes granted by a role (link table; the codes themselves are registered in code, not in the database).
CREATE TABLE IF NOT EXISTS sys_role_permission (
    role_id   BIGINT UNSIGNED NOT NULL,
    perm_code VARCHAR(128)    NOT NULL,
    PRIMARY KEY (role_id, perm_code),
    KEY idx_perm (perm_code)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- A role's data scope policy per resource type: one row per role × resource type; scope_type is the scope type code.
CREATE TABLE IF NOT EXISTS sys_role_data_scope (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    role_id       BIGINT UNSIGNED NOT NULL,
    resource_type VARCHAR(64)     NOT NULL,
    scope_type    VARCHAR(32)     NOT NULL,
    created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_role_res (role_id, resource_type),
    KEY idx_res (resource_type)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Management boundary: holders of role_id may assign assignable_role_id to others.
CREATE TABLE IF NOT EXISTS sys_role_assignable (
    role_id            BIGINT UNSIGNED NOT NULL,
    assignable_role_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (role_id, assignable_role_id),
    KEY idx_assignable (assignable_role_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Management boundary: departments a role may manage (expanded to the whole subtree when checked).
CREATE TABLE IF NOT EXISTS sys_role_manage_dept (
    role_id     BIGINT UNSIGNED NOT NULL,
    dept_id     BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (role_id, dept_id),
    KEY idx_dept (dept_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Explicit-assignment (RESOURCE_GRANT) instance rows: role × resource type × instance id.
-- Replaced as a whole in the same transaction as the policy rows of sys_role_data_scope (RoleRepo.Save).
-- The kernel does not interpret what an instance is (category, brand group, store); the option list comes from the
-- ListGrantable callback declared when the resource is registered. Like every sys_* table, only the kernel reads and
-- writes this one; after the business deletes an instance it requests cleanup through the perm.GrantRevoker port.
CREATE TABLE IF NOT EXISTS sys_role_resource_grant (
    role_id       BIGINT UNSIGNED NOT NULL,
    resource_type VARCHAR(64)     NOT NULL,
    resource_id   BIGINT UNSIGNED NOT NULL,
    created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (role_id, resource_type, resource_id),
    KEY idx_res_instance (resource_type, resource_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Audit log of write operations (append only, no delete).
CREATE TABLE IF NOT EXISTS sys_audit_log (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    operator_id  BIGINT UNSIGNED NOT NULL,
    action       VARCHAR(64)     NOT NULL,
    target_type  VARCHAR(32)     NOT NULL,
    target_id    VARCHAR(64)     NOT NULL,
    -- The request body, with sensitive fields erased per the proto's (keel.v1.redact).sensitive.
    -- No before/after snapshots of the data: only business code knows their shape, so the framework can neither
    -- redact them nor guarantee completeness; trade-off in the package comment of internal/core/audit.
    request_payload MEDIUMTEXT      NULL,
    reason       VARCHAR(255)    NOT NULL DEFAULT '',
    ip           VARCHAR(45)     NOT NULL DEFAULT '',
    created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY idx_operator (operator_id, created_at),
    KEY idx_action (action, created_at),
    KEY idx_target (target_type, target_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- Login log (append only, no delete).
CREATE TABLE IF NOT EXISTS sys_login_log (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id    BIGINT UNSIGNED NOT NULL DEFAULT 0,  -- 0 = username does not exist
    username   VARCHAR(64)     NOT NULL DEFAULT '',
    status     TINYINT(1)      NOT NULL,            -- 1 success, 0 failure
    ip         VARCHAR(45)     NOT NULL DEFAULT '',
    msg        VARCHAR(255)    NOT NULL DEFAULT '',
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY idx_user (user_id, created_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- root seed (idempotent). Initial password admin888: on every new deployment, sign in as root and change it in the
-- personal center right away. The hash is argon2id (github.com/go-keel/keel/pkg/pwd), from make root-hash PASS=admin888.
INSERT INTO sys_dept (id, parent_id, ancestors, name, sort, status)
SELECT 1, 0, '0,', 'Root', 0, 1
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM sys_dept WHERE id = 1);

INSERT INTO sys_user (username, password, nickname, dept_id, is_root, status)
SELECT 'root', '$argon2id$v=19$m=65536,t=1,p=14$WtmBy9ArYkpkHsd/eKclcw$D2bta1VTLAoxgg1+jSDpHrKiPlfdnYEqSz+dvYtyanA', 'Super Admin', 1, 1, 1
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM sys_user WHERE is_root = 1);
