"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

type PageHeader = { title: string; description?: string };
type PageHeaderContextValue = {
  header: PageHeader | null;
  setHeader: (header: PageHeader | null) => void;
};

const PageHeaderContext = createContext<PageHeaderContextValue | null>(null);

/**
 * Lets every page hand its title and description up to the sticky header, so
 * the page heading shares one row with the notification bell and account
 * controls instead of sitting on a second line below them.
 */
export function PageHeaderProvider({ children }: { children: ReactNode }) {
  const [header, setHeader] = useState<PageHeader | null>(null);
  return <PageHeaderContext.Provider value={{ header, setHeader }}>{children}</PageHeaderContext.Provider>;
}

export function usePageHeader() {
  return useContext(PageHeaderContext);
}

/**
 * The page heading. It no longer draws anything itself — it registers its
 * title/description with the surrounding header (which paints them beside the
 * bell and account menu) and clears them when the page unmounts. Pages keep
 * calling <PageIntro title description /> exactly as before.
 */
export function PageIntro({ title, description }: { title: string; description?: string }) {
  const context = usePageHeader();

  useEffect(() => {
    if (!context) return;
    context.setHeader({ title, description });
    return () => context.setHeader(null);
    // context identity is stable for the life of the provider.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [title, description]);

  // Inside a header bar (the back-office shell) the title/description ride up
  // into it, so nothing renders here. Without one (the POS portal, print
  // views) there is no bar to feed, so draw the heading inline as before.
  if (context) return null;
  return (
    <div className="mb-6 flex flex-wrap items-baseline gap-x-3 gap-y-1">
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      {description ? <p className="max-w-3xl text-sm leading-6 text-muted-foreground">{description}</p> : null}
    </div>
  );
}
