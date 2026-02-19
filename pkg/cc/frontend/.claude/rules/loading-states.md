# Loading States

## Spinner component

Use the `Spinner` component from `@/components/Spinner`:

```tsx
import Spinner from '@/components/Spinner';

// Inline spinner
<Spinner />

// Centered full-height spinner
<Spinner centered />

// With custom text
<Spinner text="Loading config" centered />
```

## Error states

- Display errors inline with red styling
- Provide actionable feedback (retry buttons, error messages)
- Use `text-error` and `bg-error/10` for error styling

## Stack progress

Use `StackProgress` for multi-phase operations with timeline visualization.
