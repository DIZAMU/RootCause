# Fonts

The fonts are self-hosted, so the dashboard loads nothing from outside the
binary. Both families are under the SIL Open Font License 1.1, which allows
bundling, and each license file sits next to its fonts.

| File | Source | SHA-256 |
|---|---|---|
| InterVariable.woff2 | Inter 4.1, `web/InterVariable.woff2` in https://github.com/rsms/inter/releases/download/v4.1/Inter-4.1.zip | 693b77d4f32ee9b8bfc995589b5fad5e99adf2832738661f5402f9978429a8e3 |
| Inter-OFL.txt | `LICENSE.txt` in the same archive | 262481e844521b326f5ecd053e59b98c8b2da78c8ee1bdbb6e8174305e54935a |
| JetBrainsMono-Regular.woff2 | JetBrains Mono 2.304, `fonts/webfonts/JetBrainsMono-Regular.woff2` in https://github.com/JetBrains/JetBrainsMono/releases/download/v2.304/JetBrainsMono-2.304.zip | a9cb1cd82332b23a47e3a1239d25d13c86d16c4220695e34b243effa999f45f2 |
| JetBrainsMono-Bold.woff2 | `fonts/webfonts/JetBrainsMono-Bold.woff2` in the same archive | c503cc5ec5f8b2c7666b7ecda1adf44bd45f2e6579b2eba0fc292150416588a2 |
| JetBrainsMono-OFL.txt | `OFL.txt` in the same archive | 30f0c136e3c88e422d0791acd97238870f9054a9729bc34cf2ff0d4ed8cac4ad |

Check them with `sha256sum web/public/assets/fonts/*.woff2 web/public/assets/fonts/*.txt`.
