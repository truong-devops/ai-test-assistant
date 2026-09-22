import type { DocumentBlock } from "@/lib/types";

export function DocumentBlockPreview({ blocks }: { blocks: DocumentBlock[] }) {
  return <div className="document-block-list">{blocks.map((block) => <article className={`document-block block-${block.block_type.toLowerCase()}`} key={block.id}>
    <header><span>Phần {block.ordinal}</span><code>{block.source_locator}</code></header>
    {block.block_type === "HEADING" ? <h3>{block.content}</h3> : block.block_type === "TABLE" ?
      <div className="table-wrap" tabIndex={0} aria-label={`Bảng tại ${block.source_locator}`}><table className="data-table"><tbody>{block.content.split("\n").filter((line) => line.trim() && !/^\s*\|?[\s:|-]+\|?\s*$/.test(line)).map((line, index) =>
        <tr key={index}>{line.replace(/^\s*\||\|\s*$/g, "").split("|").map((cell, i) => <td key={i}>{cell.trim()}</td>)}</tr>)}</tbody></table></div>
      : block.block_type === "CODE" ? <pre>{block.content}</pre> : <p className="source-text">{block.content}</p>}
  </article>)}</div>;
}
