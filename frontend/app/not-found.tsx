import Link from "next/link";
import { AppShell, EmptyState } from "@/components/shell";

export default function NotFound() {
  return <AppShell active="analyses"><EmptyState title="This record could not be found" message="It may have been removed or the address is incomplete." action={<Link className="button secondary" href="/analyses">Return to review queue</Link>} /></AppShell>;
}
