# Performance guidance

| Preset | View | Grass | Shadows | Reflections | Particles | Render scale | Post |
|---|---|---|---|---|---|---|---|
| Low | 4 chunks | 0.9 | off | sky only | 35 % | 70 % | none |
| Medium | 6 | 1.4 | 1024 / 70 m | sky only | 60 % | 85 % | bloom |
| High | 8 | 1.9 | 2048 / 110 m | planar 1/3 res | 100 % | 100 % | bloom, god rays, DOF (cinematic) |
| Ultra | 10 | 2.4 | 4096 / 150 m | planar 1/2 res | 160 % | 100 % | same |

- **Adaptive quality** lowers render scale (down to 50 %), then shadows, reflections and effects when frame time stays above ~26 ms, and restores them slowly.
- **Integrated GPUs:** use Low or Medium. Grass and shadows are the main costs; reduce draw distance in the panel for more headroom.
- **Memory:** chunks unload outside `view + 1.6`; GPU buffers are freed when chunks and landmarks unload; per-frame allocations are limited to small JS call argument slices. Tests cover bounded residency (`world`, `tests`) and pool leaks (`ecosystem`).
- **Hitches:** chunk generation is time-sliced, but a landmark is built in one go (a few ms natively; the World Tree is the largest). They load when you are still a few hundred metres away.
- **Mobile:** render scale and DPR are capped; touch controls are built in.
