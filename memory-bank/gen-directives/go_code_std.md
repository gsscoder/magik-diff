# Go Code Standards

- Generate production-grade Go (1.23+) code, favoring idiomatic, safe, and performant patterns over legacy or over-engineered abstractions
- Avoid `panic` for expected errors; wrap with `fmt.Errorf` using `%w` and check with `errors.Is`/`errors.As`
- Do not spawn unmanaged goroutines; use `errgroup` for lifecycles and always pass `context.Context` as the first parameter
- Avoid premature interface definitions; accept interfaces and return structs, defining them only in the consuming package
- Always preallocate slices and maps with `make` when capacity is known; mutate via index in `range` loops to avoid copy pitfalls
- Avoid `interface{}` and legacy `log`; use `any`, `slices`/`maps` packages, and `log/slog` for modern, type-safe operations