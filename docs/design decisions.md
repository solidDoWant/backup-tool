## Disaster recovery choices
Reasoning:
* Prefer easy restores/access to backed up data rather than simple or easy backups
* The world operates on and understands files, not always blocks or objects
* The total amount of data backed up should be relatively small, so things like optimizing restore time are not very important
* Consistency between different parts in a system (such as files and database) should be preferred wherever possible
* Processes should apply to my specific cluster, not everybody's cluster
* Processes should be reliable, maintainable, and testable

Results:
* Logical backups of DBs are prefered over physical ones, as specific DB info can be restored. If needed, the backups are also human-readable. Don't need to be restored into an identical deployment as the backup.
* Only backups for the deployment(s) I use are needed. For example, I don't need to support sqlite-backed vaultwarden, just postgres-backed vaultwarden.

## Code choices
Function parameters:
* Required parameters should either be a part of the receiver, or an explicit parameter. Optional values should be in a non-null "opts" struct.
