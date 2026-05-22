---
description: Refresh the TrustCore base coding standard from Pinecone into conventions/
---

Sync the TrustCore base coding standard from Pinecone into local canonical
memory.

1. Query the `pinecone` MCP — index `trustcore-systems`, namespace
   `coding-standards` — for the base standard records `cs-00` through `cs-10`
   (11 records).
2. Assemble them, in `cs-` order, into a single markdown document.
3. Write it with the `memory_canonical_write` tool to
   `conventions/trustcore-base-standards.md`.
4. Report what was synced.

This is a read-only pull from Pinecone — never write back to it. DevCore's own
standard remains `CODING_STANDARDS.md`; this is the general base it extends.
