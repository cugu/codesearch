<template>
  <v-app id="app" :class="{ open: open > -1 }">
    <v-main>
      <v-container fluid>
        <div style="position: relative">
          <h1 :class="titleClasses()">CODESEARCH</h1>
        </div>
        <v-row>
          <v-col cols="3" class="pt-15">
            <v-list dense>
              <v-subheader v-if="repositories.length > 0">Repository</v-subheader>
              <v-list-item-group v-model="selectedRepo" color="primary">
                <v-list-item v-for="repo in repositories" :key="repo" @click="fetchResults">
                  <v-list-item-content>
                    <v-list-item-title>{{ repo.substr(8) }}</v-list-item-title>
                  </v-list-item-content>
                </v-list-item>
              </v-list-item-group>
            </v-list>
          </v-col>
          <v-col ref="result" cols="9" class="pr-15">
            <div style="position: relative">
              <v-toolbar
                  dense
                  outlined
                  :class="searchBarClasses()"
                  elevation="0"
              >
                <v-text-field
                    hide-details
                    dense
                    class="rounded-pill"
                    solo
                    flat
                    placeholder="Search"
                    v-model="term"
                    v-on:keyup.enter="fetchResults"
                />
                <v-btn v-if="term.startsWith('https://')" icon @click="handleIndex">
                  <v-icon>mdi-download-circle-outline</v-icon>
                </v-btn>
                <v-btn icon @click="fetchResults">
                  <v-icon>mdi-magnify</v-icon>
                </v-btn>
              </v-toolbar>
            </div>
            <div v-if="loading" class="pt-15 mt-15 py-5" style="text-align: center">
              <v-progress-circular
                  indeterminate
                  color="primary"
                  class="mb-3"
              ></v-progress-circular>
              <p>load results</p>
            </div>
            <div v-else style="margin-top: 60px">
              <v-card
                  v-for="(snippet, index) in snippets"
                  :key="snippet.repo+'/'+snippet.path"
                  elevation="0"
                  style="height: 170px"
              >
                <div
                    :class="snippetClasses(index)"
                    :style="'top: ' + top(index)">
                  <v-card-title @click="closeSnippet">
                    <a target="_blank" :href="snippet.repo +'/tree/master/'+snippet.path+'#L'+(snippet.first_hit+1)">
                      {{ snippet.path }}
                    </a>
                  </v-card-title>
                  <v-card-subtitle @click="closeSnippet">
                    <a target="_blank" :href="snippet.repo +'/tree/master/'+snippet.path+'#L'+(snippet.first_hit+1)">
                      {{ snippet.repo }}
                    </a>
                    <span v-if="snippet.hit_count === 1" style="float: right">1 match</span>
                    <span v-else style="float: right">{{ snippet.hit_count }} matches</span>
                  </v-card-subtitle>
                  <v-card-text style="height: 60px">
                    <div
                        ref="snippet"
                        class="snippet-code"
                        @click="openSnippet(index)"
                        v-html="snippet.code"
                        :style="'height: ' + height(index)"/>
                  </v-card-text>
                </div>
              </v-card>
              <v-pagination
                  v-if="resultCount > 10"
                  v-model="page"
                  :length="Math.ceil(resultCount / 10)"
                  @input="fetchResults"
                  class="pt-5"
              ></v-pagination>
            </div>
          </v-col>
        </v-row>
      </v-container>
    </v-main>
    <v-overlay
        v-if="open !== -1" @click.native="closeSnippet"
        opacity="0.95"
        color="#fff"
        style="cursor: pointer">
    </v-overlay>
  </v-app>
</template>

<script>
export default {
  name: 'App',
  data: function () {
    return {
      term: "",
      searchBarTop: false,
      loading: false,
      snippets: [],
      open: -1,
      selectedRepo: undefined,
      page: 1,
      resultCount: 0,
      repositories: [],
    }
  },
  methods: {
    titleClasses: function () {
      return {
        hasresults: this.searchBarTop,
        title: true,
      }
    },
    searchBarClasses: function () {
      return {
        hasresults: this.searchBarTop,
        searchbar: true,
        "rounded-pill": true
      }
    },
    snippetClasses: function (index) {
      return {
        open: this.open === index,
        snippet: true
      }
    },
    height: function (index) {
      if (this.open === index) {
        return Math.min(this.snippets[index].maxHeight, window.innerHeight - 200) + 'px'
      }
      return "62px";
    },
    top: function (index) {
      if (this.open === index) {
        let t = this.$refs.result.scrollTop || this.$refs.result.scrollTop;
        return t + (index * -170) + 'px';
      }
      return '0px';
    },
    openSnippet: function (index) {
      if (this.open === index) {
        return
      }
      this.open = index;
      this.$nextTick(() => {
        this.$refs.snippet[index].scrollTop -= 240;
      });
    },
    closeSnippet: function () {
      let index = this.open;
      let that = this;
      that.open = -1;
      this.$nextTick(() => {
        setTimeout(function () {
          that.$refs.snippet[index].scrollTop = that.snippets[index].offset;
        }, 260);
      });
    },
    fetchResults: function () {
      if (this.term === "") {
        return
      }

      this.searchBarTop = true;
      let start = new Date();
      this.loading = true;
      this.open = -1;

      this.$nextTick(() => {
        let q = 'http://localhost:8000/search?q=' + this.term + '&offset=' + (this.page - 1) * 10;
        if (this.selectedRepo !== undefined) {
          q += '&repo=' + this.repositories[this.selectedRepo];
        }
        this.$http.get(q)
            .then((response) => {
              let wait = 0;
              let now = new Date()
              if (now - start < 250) {
                wait = 250 - (now - start)
              }
              setTimeout(() => {
                this.resultCount = response.data.count;
                this.repositories = response.data.repositories.sort();
                this.snippets = response.data.snippets;
                this.loading = false;
                this.$nextTick(() => {
                  let index;
                  for (index = 0; index < this.snippets.length; index++) {
                    let offset = this.offset(this.snippets[index].first_hit);

                    this.$refs.snippet[index].scrollTop = offset;
                    this.snippets[index].offset = offset;
                    this.snippets[index].top = this.$refs.snippet[index].getBoundingClientRect().top;
                    this.snippets[index].maxHeight = (this.snippets[index].line_count + 1) * 20 + 2;
                  }
                });
              }, wait)
            })
      })
    },
    offset: function (line) {
      if (line > 0) {
        line -= 1
      }
      return line * 20
    },
    handleIndex: function () {
      if (this.term === "" || !this.term.startsWith("https://")) {
        return
      }

      this.$http.post('http://localhost:8000/index', {url: this.term});
    }
  }
}
</script>

<style>
#app {
  font-family: Avenir, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  color: #2c3e50;
  height: 100vh;
  width: 100vw;
}

#app.open {
  overflow: hidden;
}

.col {
  height: 100vh;
  overflow-y: auto;
}

.title {
  position: absolute !important;
  top: 200px;
  left: 25%;
  padding-left: 20px;

  transition-duration: .25s !important;
  transition-property: top, left !important;
}

.title.hasresults {
  top: 20px;
  left: 8px;
  padding-left: 0;
}

.searchbar {
  position: absolute !important;
  width: 69%;
  top: 240px;

  transition-duration: .25s !important;
  transition-property: top, width !important;
}

.searchbar.hasresults {
  width: 100%;
  top: 10px;
}

.v-card__subtitle, .v-card__text, .v-card__title {
  padding-left: 0 !important;
  padding-right: 0 !important;
}

.snippet {
  position: absolute;
  width: 100%;

  transition-duration: .25s;
  transition-property: height, top;

  z-index: 4;

  padding-bottom: 3rem;
}

.open {
  /* transition-duration: .5s;
  transition-property: height, top; */
  z-index: 6;
}

.snippet .v-card__title a {
  text-decoration: none;
  color: rgba(0, 0, 0, 0.87);
}

.snippet .v-card__subtitle a {
  text-decoration: none;
  color: rgba(0, 0, 0, 0.6);
}

.snippet-code {
  height: 68px;
  line-height: 20px;

  border: 1px solid #ddd;
  border-radius: 5px;
  background-color: white;

  scroll-behavior: smooth;
  overflow: hidden;
  cursor: pointer;

  transition-duration: .25s;
  transition-property: height, top;
}

.snippet-code:hover {
  box-shadow: 0 0 5px 0 rgba(0, 0, 0, 0.25);
}

.snippet-code .ln {
  user-select: none;
}

.open .snippet-code {
  cursor: auto;
  overflow: scroll;
}

.open .v-card__title,
.open .v-card__subtitle {
  cursor: pointer;
}

.v-overlay {
  /* backdrop-filter: blur(6px); */
}
</style>
