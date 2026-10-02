# MEGA

| | |
|---|---|
| **Provider ID** | `mega` |
| **Maturity** | Supported |
| **Backend** | rclone MEGA, configured internally by ABM |

Choose **Storage → MEGA → Configure / Connect** and enter the account email and
password. ABM creates the internal remote; users never enter an rclone remote
name. The password field is masked, is not returned by any API, and rclone
obscures it in its root/Administrator-protected configuration.

MEGA is not immutable storage and its consumer backend can be less robust under
heavy parallel transfer than mature object stores. MEGA S4 is a separate
S3-compatible product with its own provider entry.
