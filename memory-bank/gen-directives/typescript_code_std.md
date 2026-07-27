# TypeScript Code Standards

- Generate code for Node.js ESM (ES2022) with TypeScript 6, favoring modern and expressive syntax over legacy patterns
- Avoid `any`; use `unknown` for uncertain types and narrow before use
- Do not use non-null assertion (`!`); handle `null`/`undefined` explicitly
- Avoid `object`/`{}`/`Record<string, any>`; use specific interfaces or type aliases
- Avoid wide unions; use discriminated unions with `type`/`kind` for explicit handling
- Avoid `as` casting; use type guards (`is`) for safe narrowing