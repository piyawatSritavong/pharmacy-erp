UPDATE branches
SET branch_type = 'main_warehouse', parent_branch_id = NULL, updated_at = NOW()
WHERE code = 'MNS';

UPDATE branches child
SET parent_branch_id = parent.id, updated_at = NOW()
FROM branches parent
WHERE parent.code = 'MNS'
  AND child.code = 'KNP'
  AND child.id <> parent.id
  AND child.branch_type = 'branch'
  AND child.parent_branch_id IS NULL;
