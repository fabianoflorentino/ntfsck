# Architecture decision records

Each ADR records one decision that shaped the code: the context, the decision,
and its consequences. The format follows Michael Nygard's
[lightweight ADR](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions).

| ADR | Decision | Status |
| --- | --- | --- |
| [0001](0001-hexagonal-architecture.md) | Use a hexagonal architecture | Accepted |
| [0002](0002-read-only-first.md) | Be read-only first; plan before writing | Accepted |

## When to add one

Add an ADR when a decision will be re-litigated — a port boundary, a package
split, an on-disk decoding shortcut, or a deliberate limitation. Use the next
number and copy the structure of an existing file. Supersede rather than edit:
mark the old one `Superseded by NNNN` and write the new one.
