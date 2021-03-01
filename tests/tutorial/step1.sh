set -e

mkdir $out/bin
echo "#!$bash
echo 'hello there!'" > $out/bin/say_hi

chmod +x $out/bin/say_hi

$out/bin/say_hi
