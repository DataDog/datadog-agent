pub mod intern;
pub mod logs;
pub mod metrics;
pub mod strategy;
pub mod tags;
pub mod trace_stats;

/// Reorder `data` according to `order` without cloning elements.
///
/// Uses the standard in-place permutation cycle algorithm: each element is
/// moved at most once, so the cost is O(n) moves with zero extra heap
/// allocations (only a scratch `Vec<bool>` for the visited set).
pub fn apply_permutation<T>(mut data: Vec<T>, order: &[usize]) -> Vec<T> {
    assert_eq!(data.len(), order.len());
    let n = data.len();
    let mut placed = vec![false; n];
    for i in 0..n {
        if placed[i] {
            continue;
        }
        let mut j = i;
        while order[j] != i {
            let target = order[j];
            data.swap(j, target);
            placed[j] = true;
            j = target;
        }
        placed[j] = true;
    }
    data
}
