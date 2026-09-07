.PHONY: check
check:
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
	rpmspec -P packages/nimbus/nimbus.spec > /dev/null
	git diff --check
