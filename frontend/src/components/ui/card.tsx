import type { HTMLAttributes, PropsWithChildren, ReactNode } from "react";

import { cn } from "@/lib/utils";

export function Card({ className, children }: PropsWithChildren<{ className?: string }>) {
  return (
    <section className={cn("min-w-0 rounded-lg border bg-card text-card-foreground shadow-sm", className)}>
      {children}
    </section>
  );
}

export function CardHeader({
  title,
  description,
  actions,
  className
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex flex-col gap-2 border-b px-3 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-3 sm:px-6 sm:py-4",
        className
      )}
    >
      <div className="min-w-0 space-y-1">
        <h2 className="text-base font-semibold tracking-tight">{title}</h2>
        {description ? <p className="text-sm text-muted-foreground">{description}</p> : null}
      </div>
      {actions}
    </div>
  );
}

export function CardBody({
  className,
  children,
  ...props
}: PropsWithChildren<HTMLAttributes<HTMLDivElement>>) {
  return (
    <div className={cn("min-w-0 p-3 sm:p-6", className)} {...props}>
      {children}
    </div>
  );
}
