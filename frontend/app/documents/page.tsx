import Link from "next/link";
import { CreateDocumentSet } from "@/components/create-document-set";
import { AppShell, EmptyState, PageHeading } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { getDocumentSets } from "@/lib/api";
import { formatDate } from "@/lib/presentation";

export const dynamic = "force-dynamic";

export default async function DocumentsPage() {
  const sets = await getDocumentSets();
  return (
    <AppShell active="documents">
      <PageHeading eyebrow="Authoritative product evidence" title="Document sources" description="Create a scope, upload independently authored product documents, and inspect exactly what the parser extracted." />
      <CreateDocumentSet />
      {sets.length ? (
        <section className="panel">
          <div className="panel-header"><div><h2>Document sets</h2><p>Each set is an isolated source boundary for future requirements and test cases.</p></div><span className="section-counter">{sets.length} set{sets.length === 1 ? "" : "s"}</span></div>
          <div className="table-wrap"><table className="data-table">
            <thead><tr><th>Set</th><th>Status</th><th>Created</th></tr></thead>
            <tbody>{sets.map((set) => <tr key={set.id}>
              <td><Link href={`/documents/${set.id}`}><span className="table-title">{set.name}</span><span className="table-subtitle">{[set.product_name, set.scope, set.description].filter(Boolean).join(" · ") || "No description"}</span></Link></td>
              <td><StatusBadge status={set.status} /></td><td>{formatDate(set.created_at)}</td>
            </tr>)}</tbody>
          </table></div>
        </section>
      ) : <EmptyState title="No document sets yet" message="Create the first source set above, then upload requirements, user stories, designs, or historical defect records." />}
    </AppShell>
  );
}
