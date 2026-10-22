# Process & Progress Log

## 🟢 Resume Point

> Read this section first when returning to the project after a long break.

### Where I stopped

กำลังพัฒนาหน้า Satoyama Developer Website

### What I was doing

กำลังนำ prototype `satoyama_portfolio.html`
มาพิจารณา integrate เข้ากับโครงสร้าง `web/`

### Last known working state

- USB printer filtering ทำงานแล้ว
- Satoyama portfolio prototype เปิดใช้งานได้
- ยังไม่ได้ integrate portfolio เข้าระบบ production

### Next action

1. ตรวจสอบโครงสร้าง `web/`
2. ดูว่าหน้า portfolio ควรแยกเป็น section หรือ standalone page
3. ตรวจสอบ responsive layout
4. ค่อยเริ่ม integrate

### Important things to remember

- อย่าแก้ database/API โดยไม่จำเป็น
- `web/sections/detail.html` มี logic USB printer filtering
- `satoyama_portfolio.html` ยังเป็น prototype

## Current State

- **Status:** Computer Inventory USB printer filtering updated.
- **Current Phase:** Hospital IT Operations layer (Health, Policy, Agent lifecycle, Governance)
- **Last Updated:** 2026-09-15

### Operations Layer Progress

- Added agent version reporting and device health scoring.
- Added security-aware health evaluation, health change timeline entries, policy settings API/UI, and backup visibility API/UI.
- Ticketing remains intentionally deferred; the data model is not exposed to users yet.

### Monitor Size Fix

- Added the missing `size_inch` migration/column compatibility. The API and Inventory column now work with databases created from the older `size_cm` schema.

### Storage Capacity Collection Fix

- Added a `psutil` fallback for logical volume usage so Used/Free capacity is still collected when WMI is unavailable or restricted.
- Kept WMI as the fallback collector for Windows volume labels and drive metadata.

### Visual Polish Pass

- Added a shared visual polish layer for cards, tables, buttons, forms, navigation, focus states, badges, and small-screen spacing.
- Kept the existing Satoyama pastel theme and page structure intact.

### Storage Capacity Display Fallback

- Storage Capacity now falls back to physical SSD/HDD inventory when logical volume Used/Free data has not arrived from the Agent yet.
- This prevents the section from appearing empty while clearly marking unavailable Used/Free values.
- Added a PowerShell/CIM fallback and Agent logging for the number of logical volumes collected.

### Device Building / Department Assignment Fix

- Saving Building or Department in `Edit Computer & Asset Info` now creates or reuses the matching location record when no room is selected.
- The device list and edit form can therefore retrieve the selected Building / Department consistently.

## Last Action

- Simplified the Satoyama developer website prototypes.
   - Removed animated clouds, firefly particles, pulsing elements, and animated card hover effects.
   - Kept the static layout, content, and Satoyama color theme.

1. Updated `web/sections/detail.html`
   - Added USB printer filtering.
   - Only USB-connected printers are displayed under `Connected USB Printers`.

2. Created Satoyama developer website prototype.
   - `satoyama_portfolio.html`
   - `web/satoyama_portfolio.html`

## Pending Tasks

- [ ] None

## Completed Tasks

- [x] USB printer filtering
- [x] Satoyama developer website prototype

## Technical Changes

### USB Printer Detail Filtering

File:

`web/sections/detail.html`

Filter logic:

- `item.type.includes('USB')`
- `item.printer_type.includes('USB')`
- `port_name.includes('USB')`

Purpose:

Only display printers connected through USB.

### Satoyama Developer Website

Files:

- `c:\project A\satoyama_portfolio.html`
- `web/satoyama_portfolio.html`

Theme:

- Satoyama Edition
- Anime Cozy
- Warm cream / forest green / sky blue

Colors:

- `#FDFBF7` — Warm Cream
- `#2D5A27` — Satoyama Forest Green
- `#7DA0FA` — Sky Blue
- `#FFC83B` — Firefly Gold
- `#2C3E50` — Dark Charcoal

Features:

- Status badge
- Feature cards
- Signature footer
- Static layout without decorative animation effects

## Decisions

- Preserve existing project architecture.
- Avoid modifying unrelated files.
- Keep prototypes separate from production components.

## Next Steps

- [ ] Decide whether `satoyama_portfolio.html` becomes the production developer website.
- [ ] Integrate prototype into existing web structure.
- [ ] Review responsive layout.
