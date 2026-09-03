export type NavigationItem = {
  key: string;
  title: string;
  href: string;
  description: string;
  /** True for a feature that lives behind PharmaPOS Pro — the nav shows it with
   *  a "Pro" badge, and the page itself renders the upgrade gate. */
  pro?: boolean;
  /** Present on parent/group menu items (C1) — href points at children[0]. */
  children?: NavigationItem[];
};

export type Session = {
  user: {
    id: string;
    name: string;
    email: string;
    role_key: string;
    role_name: string;
    /** "backoffice" or "pos" (D11) — which app shell this role renders. */
    portal: string;
    /** "global" or "branch" (D11) — whether the role is tied to one branch. */
    scope: string;
    branch_id?: string;
    branch_name?: string;
    permissions: string[];
  };
  navigation: NavigationItem[];
  home_path: string;
};

/** A role preset as returned by GET /roles (D11). */
export type Role = {
  id: string;
  role_key: string;
  name: string;
  active: boolean;
  is_system: boolean;
  portal: string;
  scope: string;
  permissions: string[];
};

/** A permission catalog entry as returned by GET /permissions (D11). */
export type Permission = {
  permission_key: string;
  name: string;
  description: string;
};
