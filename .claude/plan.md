# Plan: Shopify-Style Frontend Implementation

## Overview
Rewrite the entire frontend to match Shopify's admin interface design (Polaris design system) using the existing page routes and APIs. Remove Chakra UI dependency, use plain CSS with Shopify design patterns.

## Changes

### 1. Remove Chakra UI + framer-motion dependencies
- Replace with lightweight custom components
- Use a single CSS file (`shopify.css`) with Shopify Polaris design tokens

### 2. Install react-hot-toach for toast notifications
- All errors show as toasts (bottom-right), no inline error messages
- No mock data, no fallback patterns

### 3. Shopify Design System CSS
Create `src/app/shopify.css` with tokens matching Shopify Polaris:
- Colors: #008060 (green primary), #202223 (text), #6d7175 (subdued), #e4e5e7 (border), #f6f6f7 (bg-subdued)
- Typography: -apple-system, BlinkMacSystemFont, 'Segoe UI' 
- Components: buttons, cards, data tables, breadcrumbs, badges, inputs
- Layout: left sidebar nav, main content area (Shopify admin layout)

### 4. File Changes

#### REWRITE - All pages get Shopify admin layout

| File | Action |
|------|--------|
| `src/lib/api.ts` | Fix batchGetSkus — use POST body not URL params; fix getOrder endpoint |
| `src/app/globals.css` | Replace with Shopify base styles (keep minimal reset) |
| `src/app/shopify.css` | NEW — Shopify Polaris design tokens and component classes |
| `src/app/layout.tsx` | Root layout — no Chakra, load shopify.css |
| `src/app/providers.tsx` | REMOVE — wrap Toaster in layout instead |
| `src/app/page.tsx` | Landing page in Shopify style |
| `src/app/login/page.tsx` | Shopify-style login form (full page, centered card) |
| `src/app/register/page.tsx` | Shopify-style register form |
| `src/app/products/page.tsx` | Product list as data table with status badges, create button |
| `src/app/products/new/page.tsx` | NEW — Create product form (name, SKUs, prices) |
| `src/app/products/[id]/page.tsx` | NEW — Edit product page using PATCH endpoint |
| `src/app/cart/page.tsx` | Cart with line items, quantity controls, checkout |
| `src/app/orders/page.tsx` | Order list as data table with status filters |
| `src/app/orders/[id]/page.tsx` | Order detail with items table, pay/cancel actions |
| `src/app/inventory/page.tsx` | Inventory data table with stock adjust inline |
| `src/app/payment/page.tsx` | Payment confirmation page |

#### REMOVE - Unused components
- `src/components/Navbar.tsx` — replaced by Shopify sidebar layout
- `src/components/ProductCard.tsx` — replaced by product table
- `src/components/CartItem.tsx` — inline in cart page
- `src/components/Button.tsx` — replaced by CSS button classes
- `src/components/OrderCard.tsx` — replaced by order table
- `src/app/providers.tsx` — Chakra provider removed

### 5. Install react-hot-toast
```bash
npm install react-hot-toast
```

### 6. Key Implementation Details

**Auth pages**: Centered card on white page, clean form with proper labels, toast on error

**Products page**: Data table showing ID, name, status (colored badge), SKU count, actions (edit). 
- Filter by status via search params
- Create Product page with dynamic SKU rows (attrs as JSON, unit price)
- Edit Product page pre-fills from API, uses BatchUpdateProduct

**Cart page**: Line items with product name, SKU, unit price, qty controls, line total. 
- Summary sidebar showing subtotal, total
- Checkout button creates order via API, redirects to order detail

**Orders page**: Data table with order ID, status badge, item count, total, date
- Filter by status
- Click to order detail

**Order detail**: Items breakdown table, order total, status badge
- Pay Now button (calls CreatePayment → redirects to payment URL)
- Cancel button (POST cancel)

**Inventory page**: Data table with SKU ID, product name, stock qty, available, reserved
- Inline "Set" input to adjust stock
- Refresh button

**Payment page**: Shows order info, payment method selector, confirm button
- On success redirects to order detail

### 7. API Fixes
- `batchGetSkus` needs POST body not URL params (match proto spec)
- `getOrder` missing from the underlying request function — need to add `GET /v1/orders/{order_id}` mapping
