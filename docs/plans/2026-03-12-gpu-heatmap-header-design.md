# GPU Heatmap Header Design

**Goal:** Place the `GPU 热力图` title and the summary metrics on the same modal title row, while keeping the summary itself as a single horizontal line.

**Design:**
- Keep the modal title as the single visible heatmap title to avoid duplicate headings inside the heatmap content.
- Move the summary rendering into a reusable header summary view that can be shown inside the modal title once the heatmap data has loaded.
- Let `AcceleratorHeatmap` publish fresh summary data upward through a callback so the modal title stays in sync with refreshes.

**Testing:**
- Add a dashboard test that opens the heatmap modal and verifies the title row shows `GPU 热力图`, `总卡数量`, and `占用量`.
- Run the targeted dashboard and heatmap tests after implementation.
