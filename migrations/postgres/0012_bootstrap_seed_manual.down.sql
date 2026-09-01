-- Reverses the bootstrap seed.
--
-- It deletes BY THE FIXED IDS the up migration wrote, never by value: a tenant
-- someone later named `master`, or a permission row an operator recreated after
-- archiving the seeded one, carries a different id and survives this file.
--
-- Order is child before parent, because role_permissions and user_roles hold
-- foreign keys into roles, permissions and users.

DELETE FROM "user_roles"
 WHERE "id" = '01990000-0005-7000-8000-000000000001';

DELETE FROM "users"
 WHERE "id" = '01990000-0004-7000-8000-000000000001';

DELETE FROM "role_permissions"
 WHERE "id" = '01990000-0003-7000-8000-000000000001';

DELETE FROM "roles"
 WHERE "id" = '01990000-0002-7000-8000-000000000001';

DELETE FROM "tenants"
 WHERE "id" = '01990000-0001-7000-8000-000000000001';

-- The whole catalog, wildcard included: ids 00 through 26 of the seed block.
DELETE FROM "permissions"
 WHERE "id" BETWEEN '01990000-0000-7000-8000-000000000000'
                AND '01990000-0000-7000-8000-000000000026';
