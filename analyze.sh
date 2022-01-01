#!/bin/sh

# ./analyze 改善しました 100


KATARU=$(make kataru)
SLOW=$(make slow)

sudo gh issue comment 1 --body "
## $1 $2

<details>
<summary>kataribe</summary>

```
$KATARU
```
</details>

<details>
<summary>slow-log</summary>

```
$SLOW
```
</details>

$(git rev-parse HEAD)
"
