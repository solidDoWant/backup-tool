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

## Configuration and parsing
Desired result:
* Be able to parse and validate YAML files for each DR provider. Also be able to produce a JSON schema file to "shift left" validation of files.
* Support for other configuration sources (such as env vars) is not needed.
* Basic validation support (such as required fields, enums) is required.

Comments:
For such a heavily opinionated language, the Go ecosystem has an enormous number of ways to parse config files. Most parser projects have very minimal difference between them.
Validation is spotty at best, and options for producing schema files is nearly zero.

Struct tags are very inconsistent between librarys. For example, basically every parser library has a different way to represent a field as required.

Solution:
* Tag structs with tags from https://github.com/invopop/jsonschema for schema-specific information, such as description
* Tag structs with tags from https://github.com/go-playground/validator for actual validation
* Use https://github.com/fatih/structtag to convert validator tags to json schema tags when producing a json schema file
* Load config with https://github.com/goccy/go-yaml, validating with https://github.com/go-playground/validator
