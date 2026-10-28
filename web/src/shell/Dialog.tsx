import { useEffect, useRef, type ReactNode } from "react";

// Dialog is a modal <dialog>: showModal keeps focus inside it and makes the
// rest of the page inert; Esc closes it. On close, focus goes back to the
// element that had it before.
export function Dialog({
  open,
  onClose,
  labelledBy,
  className,
  children,
}: {
  open: boolean;
  onClose: () => void;
  labelledBy: string;
  className?: string;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const opener = useRef<Element | null>(null);
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      opener.current = document.activeElement;
      d.showModal();
    } else if (!open && d.open) {
      d.close();
    }
  }, [open]);
  return (
    <dialog
      ref={ref}
      className={className ? `dialog ${className}` : "dialog"}
      aria-labelledby={labelledBy}
      onClose={() => {
        onClose();
        const back = opener.current;
        if (back instanceof HTMLElement && back.isConnected) back.focus();
      }}
      onClick={(e) => {
        // A click on the backdrop (the dialog element itself) closes it.
        if (e.target === e.currentTarget) e.currentTarget.close();
      }}
    >
      {children}
    </dialog>
  );
}
