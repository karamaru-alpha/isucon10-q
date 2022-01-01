#!/bin/sh

# ./analyze 改善しました 100

sudo gh issue comment 1 --body "
<details>
<summary>kataribe</summary>

\`\`\`
$(make kataru)
\`\`\`
</details>

<details>
<summary>slow-log</summary>

\`\`\`
$(make slow)
\`\`\`
</details>
"
