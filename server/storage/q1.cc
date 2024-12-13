#include <iostream>
#include <fstream>
#include <vector>
#include <algorithm>
#include <cassert>
using namespace std;

class my_tree_base {
public:
    virtual int max_node() = 0;
    virtual void build_graph(vector<vector<int>>& adj, int& next_vertex) = 0;
    virtual int source() = 0;
    virtual int sink() = 0;
};

class leaf : public my_tree_base {
public:
    int x, y;

    int max_node() override {
        return max(x, y);
    }

    void build_graph(vector<vector<int>>& adj, int& next_vertex) override {
        adj[x].push_back(y);
        adj[y].push_back(x);
    }

    int source() override { return x; }
    int sink() override { return y; }
};

class parallel : public my_tree_base {
public:
    int a, b;
    my_tree_base* left;
    my_tree_base* right;

    int max_node() override {
        return max(left->max_node(), right->max_node());
    }

    void build_graph(vector<vector<int>>& adj, int& next_vertex) override {
        left->build_graph(adj, next_vertex);
        right->build_graph(adj, next_vertex);
    }

    int source() override { return a; }
    int sink() override { return b; }
};

class series : public my_tree_base {
public:
    int a, b, c;
    my_tree_base* left;
    my_tree_base* right;

    int max_node() override {
        return max(left->max_node(), right->max_node());
    }

    void build_graph(vector<vector<int>>& adj, int& next_vertex) override {
        left->build_graph(adj, next_vertex);
        right->build_graph(adj, next_vertex);
    }

    int source() override { return a; }
    int sink() override { return c; }
};

my_tree_base* read_tree(istream& in, int& next_vertex) {
    char c;
    in >> c;

    if (c == '(') {
        in >> c;
        if (c == 'L') {
            int x, y;
            in >> x >> y >> c;
            leaf* p = new leaf();
            p->x = x;
            p->y = y;
            next_vertex = max(next_vertex, max(x, y) + 1);
            return p;
        } else if (c == 'P') {
            int x, y;
            in >> x >> y;
            parallel* p = new parallel();
            p->a = x;
            p->b = y;
            p->left = read_tree(in, next_vertex);
            p->right = read_tree(in, next_vertex);
            in >> c;
            next_vertex = max(next_vertex, max(p->a, p->b) + 1);
            return p;
        } else if (c == 'S') {
            int x, y, z;
            in >> x >> y >> z;
            series* p = new series();
            p->a = x;
            p->b = y;
            p->c = z;
            p->left = read_tree(in, next_vertex);
            p->right = read_tree(in, next_vertex);
            in >> c;
            next_vertex = max(next_vertex, max(p->a, max(p->b, p->c)) + 1);
            return p;
        }
    }
    return nullptr;
}

// SP decomposition to nice-tree decomposition
my_tree_base* toNiceTree(my_tree_base* root) {
    if (dynamic_cast<leaf*>(root)) {
        return root; 
    } else if (dynamic_cast<parallel*>(root)) {
        parallel* p = dynamic_cast<parallel*>(root);
        p->left = toNiceTree(p->left); 
        p->right = toNiceTree(p->right);
        return p;  
    } else if (dynamic_cast<series*>(root)) {
        series* s = dynamic_cast<series*>(root);
        s->left = toNiceTree(s->left);  
        s->right = toNiceTree(s->right);
        return s;  
    }
    return nullptr;
}

// DP Function for solving MWIS on tree structure
int dp_solve(int v, const vector<vector<int>>& adj, const vector<int>& weights, vector<vector<int>>& dp, vector<vector<bool>>& dp_choice, vector<bool>& visited) {
    if (visited[v]) return dp[v][0];
    
    visited[v] = true;
    dp[v][0] = 0;  // Not including v in the MWIS
    dp[v][1] = weights[v];  // Including v in the MWIS
    
    for (int u : adj[v]) {
        if (!visited[u]) {
            dp_solve(u, adj, weights, dp, dp_choice, visited); 
            dp[v][0] += max(dp[u][0], dp[u][1]); // If v is excluded, we take the max of each neighbor (either include or exclude neighbor)
            dp[v][1] += dp[u][0]; // If v is included, we exclude its neighbors
        }
    }

    return max(dp[v][0], dp[v][1]);  // Return the max of the two possibilities
}

// Backtrack to get the vertices in the MWIS
vector<int> backtrack(int v, const vector<vector<int>>& adj, vector<vector<bool>>& dp_choice, vector<bool>& visited) {
    vector<int> mwis_vertices;
    
    // If the vertex is included in the MWIS
    if (dp_choice[v][1]) {
        mwis_vertices.push_back(v);
        for (int u : adj[v]) {
            // Skip the adjacent vertex if it is part of the independent set
            if (visited[u] && dp_choice[u][0]) {
                backtrack(u, adj, dp_choice, visited); 
            }
        }
    } else {
        for (int u : adj[v]) {
            if (visited[u] && dp_choice[u][0]) {
                backtrack(u, adj, dp_choice, visited); 
            }
        }
    }

    return mwis_vertices;
}

int solve_mwis(vector<vector<int>>& adj, vector<int>& weights) {
    int n = adj.size();
    vector<vector<int>> dp(n, vector<int>(2, 0));  // DP table for including or excluding a vertex
    vector<vector<bool>> dp_choice(n, vector<bool>(2, false));  // To track choices for backtracking
    vector<bool> visited(n, false);  // To keep track of visited nodes

    dp_solve(0, adj, weights, dp, dp_choice, visited); 
    
    vector<int> mwis_vertices = backtrack(0, adj, dp_choice, visited);
    
    cout << "Vertices in the Maximum Weighted Independent Set: ";
    for (int v : mwis_vertices) {
        cout << v << " ";
    }
    cout << endl;
    
    return dp[0][0];  // Return the MWIS value (max of including or excluding the root)
}

int main(int argc, char *argv[]) {
    if (argc < 3) {
        cout << "Usage: ./program sp1.dat w1.dat" << endl;
        return -1;
    }

    ifstream treeFile(argv[1]);
    ifstream weightsFile(argv[2]);

    if (!treeFile.is_open()) {
        cout << "Error opening file " << argv[1] << endl;
        return -1;
    }
    if (!weightsFile.is_open()) {
        cout << "Error opening file " << argv[2] << endl;
        return -1;
    }

    int next_vertex = 0;
    my_tree_base* root = read_tree(treeFile, next_vertex);

    my_tree_base* nice_tree_root = toNiceTree(root);

    vector<vector<int>> adj(next_vertex);
    nice_tree_root->build_graph(adj, next_vertex);

    vector<int> weights(next_vertex);
    for (int i = 0; i < next_vertex; ++i) {
        weightsFile >> weights[i];
    }

    int result = solve_mwis(adj, weights);
    cout << "Maximum Weighted Independent Set: " << result << endl;

    return 0;
}
