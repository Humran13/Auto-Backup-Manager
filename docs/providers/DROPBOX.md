# Dropbox

| | |
|---|---|
| **Provider ID** | `dropbox` |
| **Maturity** | Stable |
| **Backend** | rclone Dropbox, managed internally by ABM |

Choose **Storage → Dropbox → Connect**, authorize in the provider browser page,
and return to ABM. The resulting token is kept in ABM's protected rclone
configuration and is never shown in the GUI or API. Public CI tests the
rendered connection workflow with the external login boundary unfulfilled;
real-account testing is optional and credential-gated.
