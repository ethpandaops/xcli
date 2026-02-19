# Storybook

## Co-location

Stories are co-located with their components:

```
ComponentName/
  ComponentName.tsx
  ComponentName.stories.tsx
  index.ts
```

## Decorators

Add a background wrapper decorator to new stories:

```tsx
decorators: [
  Story => (
    <div className="bg-bg p-8">
      <Story />
    </div>
  ),
],
```

## Story titles

Use the component path:

```
Components/Dashboard
Components/ServiceCard
Components/Spinner
```

## Theme toggle

Storybook toolbar includes a theme toggle (light/dark) via `@storybook/addon-themes`.

## MSW

Mock Service Worker is available for API mocking:

```tsx
parameters: {
  msw: {
    handlers: [
      http.get('/api/v1/endpoint', () => HttpResponse.json({ ... })),
    ],
  },
},
```
