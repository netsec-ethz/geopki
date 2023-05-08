FILES = geopki
FILES = $(patsubst %.tex,%,$(wildcard *.tex))
FILES_PDF = $(addsuffix .pdf,$(FILES))

# You want latexmk to *always* run, because make does not have all the info.
# Also, include non-file targets in .PHONY so they are run regardless of any
# file of the given name existing.
.PHONY: $(FILES_PDF) all clean

# The first rule in a Makefile is the one executed by default ("make"). It
# should always be the "all" rule, so that "make" and "make all" are identical.
all: $(FILES_PDF) $(FILES_FINAL_PDF)

$(FILES_PDF): %.pdf: %.tex
	latexmk -pdf -jobname=$(basename $@) -bibtex --synctex=1 -shell-escape $<

clean:
	latexmk -c
	$(RM) *.log
	$(RM) -r *.prv
	$(RM) *.bbl
	$(RM) *-diff.tex
	$(RM) *.synctex.gz
	$(RM) *.fdb_latexmk
	$(RM) *.fls
	$(RM) *.aux
	$(RM) *.blg
	$(RM) *.out
	$(RM) *.nav
	$(RM) *.dvi
	$(RM) *.snm
