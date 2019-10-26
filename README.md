## Golang Cafe

[Golang Cafe](https://golang.cafe) is the first Go job board with no recruiters and clear salary ranges. It's a curated Go Job Board with hand-picked Go jobs where Go engineers can directly apply to companies.

### Tech Stack

The web app is written in [Go](https://golang.org)/[HTML](https://www.w3.org/html/)/[CSS](https://developer.mozilla.org/en-US/docs/Web/CSS)/[JavaScript](https://developer.mozilla.org/en-US/docs/Web/JavaScript). The project started as a prototype with a Google Form as submission page, a Google Sheet as database and a simple page that pulled data from the spreadsheet and displayed in a dead simple HTML page.

As of today the app is written in Go using [PostgreSQL](https://www.postgresql.org) as primary data store and it's being hosted on [Heroku](https://heroku.com). No frameworks have been used, apart from Go's [gorilla mux](https://github.com/gorilla/mux) for routing. The frontend is written in vanilla JavaScript using a class-less CSS framework called [tacit](https://yegor256.github.io/tacit/).

### License

This source code is licensed under [BSD 3-Clause License](LICENSE.txt) 
