export type NavigationItem = {
  key: string;
  title: string;
  href: string;
  description: string;
};

export type Session = {
  user: {
    id: string;
    name: string;
    email: string;
    role_key: string;
    role_name: string;
    branch_id?: string;
    branch_name?: string;
    permissions: string[];
  };
  navigation: NavigationItem[];
  home_path: string;
};
