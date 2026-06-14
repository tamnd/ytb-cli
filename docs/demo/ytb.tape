# Demo tape for ytb. Rendered with ascii-gif (github.com/tamnd/ascii-gif),
# which supplies the window chrome and theme; this file is just the action.
#
#   ascii-gif render docs/demo/ytb.tape -o docs/static/demo.gif
#
# ytb must be on PATH inside the recording shell.

Hide
Type "export PS1='$ ' PATH=/Users/apple/bin:$PATH"
Enter
Type "clear"
Enter
Show

Sleep 500ms
Type "ytb search 'lofi hip hop' -n 5 --fields id,title,channel,views"
Sleep 800ms
Enter
Sleep 3s

Type "ytb video dQw4w9WgXcQ --fields title,channel,views,duration"
Sleep 800ms
Enter
Sleep 2.5s

Type "ytb search 'lofi hip hop' -n 4 -o jsonl | jq -r .title"
Sleep 800ms
Enter
Sleep 3s
