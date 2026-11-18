# Demo recording for the README. Regenerate with "make demo" (needs Docker).
# The vault is imported from docs/demo/vault.json (made-up profiles) into a
# temporary HOME; nothing real is used.

Output docs/demo/mycnfm.gif

Set Shell "bash"
Set FontSize 14
Set Width 1100
Set Height 620
Set Padding 18
Set Theme "Dracula"
Set TypingSpeed 60ms
Set Framerate 24

Hide
Type "export PATH=$PWD:$PATH HOME=/tmp/demo PS1='$ ' MYCNFM_PASSWORD=demo D=$PWD; mkdir -p $HOME && cd $HOME" Enter
Type "mycnfm import -json $D/docs/demo/vault.json" Enter
Sleep 6s
Type "demo" Enter
Sleep 2s
Type "demo" Enter
Sleep 2s
Type "clear" Enter
Sleep 1s
Show

Type "mycnfm" Sleep 400ms Enter
Sleep 2.5s
Down Sleep 700ms
Enter Sleep 1.5s
Down Sleep 700ms
Down Sleep 700ms
Up Sleep 700ms
Up Sleep 700ms
Enter Sleep 3s
Ctrl+R Sleep 2s
Ctrl+R Sleep 1s
Type "v" Sleep 2.5s
Type "v" Sleep 1s
Escape Sleep 1s
Type "/" Sleep 500ms
Type "auth" Sleep 2.5s
Escape Sleep 1s
Type "q" Sleep 1s

Type "cat ~/.my.cnf" Sleep 400ms Enter
Sleep 3.5s
Type "mycnfm clean && cat ~/.my.cnf" Sleep 400ms Enter
Sleep 3s
